/*
Copyright 2019 yametech.
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var controllerKind = nuwav1.GroupVersion.WithKind("StatefulSet")

// StatefulSetControl implements the control logic for updating StatefulSets and their children Pods. It is implemented
// as an interface to allow for extensions that provide different semantics. Currently, there is only one implementation.
type StatefulSetControlInterface interface {
	// UpdateStatefulSet implements the control logic for Pod creation, update, and deletion, and
	// persistent volume creation, update, and deletion.
	// If an implementation returns a non-nil error, the invocation will be retried using a rate-limited strategy.
	// Implementors should sink any errors that they do not wish to trigger a retry, and they may feel free to
	// exit exceptionally at any point provided they wish the update to be re-run at a later point in time.
	UpdateStatefulSet(set *nuwav1.StatefulSet, pods []*corev1.Pod) error
	// ListRevisions returns a array of the ControllerRevisions that represent the revisions of set. If the returned
	// error is nil, the returns slice of ControllerRevisions is valid.
	ListRevisions(set *nuwav1.StatefulSet) ([]*appsv1.ControllerRevision, error)
	// AdoptOrphanRevisions adopts any orphaned ControllerRevisions that match set's Selector. If all adoptions are
	// successful the returned error is nil.
	AdoptOrphanRevisions(set *nuwav1.StatefulSet, revisions []*appsv1.ControllerRevision) error
}

// StatefulPodControlInterface defines the interface that StatefulSetController uses to create, update, and delete Pods,
// and to update the Status of a StatefulSet. It follows the design paradigms used for PodControl, but its
// implementation provides for PVC creation, ordered Pod creation, ordered Pod termination, and Pod identity enforcement.
// Like controller.PodControlInterface, it is implemented as an interface to provide for testing fakes.
type StatefulPodControlInterface interface {
	// CreateStatefulPod create a Pod in a StatefulSet. Any PVCs necessary for the Pod are created prior to creating
	// the Pod. If the returned error is nil the Pod and its PVCs have been created.
	CreateStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error
	// UpdateStatefulPod Updates a Pod in a StatefulSet. If the Pod already has the correct identity and stable
	// storage this method is a no-op. If the Pod must be mutated to conform to the Set, it is mutated and updated.
	// pod is an in-out parameter, and any updates made to the pod are reflected as mutations to this parameter. If
	// the create is successful, the returned error is nil.
	UpdateStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error
	// DeleteStatefulPod deletes a Pod in a StatefulSet. The pods PVCs are not deleted. If the delete is successful,
	// the returned error is nil.
	DeleteStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error
}

// StatefulSetReconciler reconciles a StatefulSet object
type StatefulSetReconciler struct {
	// Default
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	// control returns an interface capable of syncing a stateful set.
	// Abstracted out for testing.
	control StatefulSetControlInterface
	// podControl is used for patching pods.
	podControl controller.PodControlInterface
	// queue
	queue workqueue.RateLimitingInterface
}

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets/status,verbs=get;update;patch
func (r *StatefulSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("statefulset", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func (r *StatefulSetReconciler) CreateStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error {
	// Create the Pod's PVCs prior to creating the Pod
	if err := r.createPersistentVolumeClaims(set, pod); err != nil {
		//r.recordPodEvent("create", set, pod, err)
		return err
	}
	// If we created the PVCs attempt to create the Pod
	pod.Namespace = set.Namespace
	err := r.Client.Create(context.TODO(), pod)
	// sink already exists errors
	if apierrors.IsAlreadyExists(err) {
		return err
	}
	//r.recordPodEvent("create", set, pod, err)
	return err
}

func (r *StatefulSetReconciler) UpdateStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error {
	attemptedUpdate := false
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// assume the Pod is consistent
		consistent := true
		// if the Pod does not conform to its identity, update the identity and dirty the Pod
		if !identityMatches(set, pod) {
			updateIdentity(set, pod)
			consistent = false
		}
		// if the Pod does not conform to the StatefulSet's storage requirements, update the Pod's PVC's,
		// dirty the Pod, and create any missing PVCs
		if !storageMatches(set, pod) {
			updateStorage(set, pod)
			consistent = false
			if err := r.createPersistentVolumeClaims(set, pod); err != nil {
				//r.recordPodEvent("update", set, pod, err)
				return err
			}
		}
		// if the Pod is not dirty, do nothing
		if consistent {
			return nil
		}

		attemptedUpdate = true
		// commit the update, retrying on conflicts
		pod.Namespace = set.Namespace
		updateErr := r.Client.Update(context.TODO(), pod)
		if updateErr == nil {
			return nil
		}

		updated := &corev1.Pod{}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: set.Namespace, Name: pod.Name}, updated); err == nil {
			// make a copy so we don't mutate the shared cache
			pod = updated.DeepCopy()
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated Pod %s/%s from lister: %v", set.Namespace, pod.Name, err))
		}

		return updateErr
	})
	if attemptedUpdate {
		//r.recordPodEvent("update", set, pod, err)
	}
	return err
}

func (r *StatefulSetReconciler) DeleteStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error {
	pod.Namespace = set.Namespace
	err := r.Client.Delete(context.TODO(), pod)
	return err
}

// createPersistentVolumeClaims creates all of the required PersistentVolumeClaims for pod, which must be a member of
// set. If all of the claims for Pod are successfully created, the returned error is nil. If creation fails, this method
// may be called again until no error is returned, indicating the PersistentVolumeClaims for pod are consistent with
// set's Spec.
func (r *StatefulSetReconciler) createPersistentVolumeClaims(set *nuwav1.StatefulSet, pod *corev1.Pod) error {
	var errs []error

	for _, claim := range getPersistentVolumeClaims(set, pod) {
		objKey := types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name}
		err := r.Client.Get(context.TODO(), objKey, &claim)
		switch {
		case apierrors.IsNotFound(err):
			//TODO
			err := r.Client.Create(context.TODO(), &claim)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to create PVC %s: %s", claim.Name, err))
			}
			if err == nil || !apierrors.IsAlreadyExists(err) {
				//r.recordClaimEvent("create", set, pod, &claim, err)
			}
		case err != nil:
			errs = append(errs, fmt.Errorf("failed to retrieve PVC %s: %s", claim.Name, err))
			//r.recordClaimEvent("create", set, pod, &claim, err)
		}
		// TODO: Check resource requirements and accessmodes, update if necessary
	}
	return errors.NewAggregate(errs)
}

var _ StatefulPodControlInterface = &StatefulSetReconciler{}

// addPod adds the statefulset for the pod to the sync queue
func (r *StatefulSetReconciler) addPod(obj interface{}) {
	pod := obj.(*corev1.Pod)

	if pod.DeletionTimestamp != nil {
		// on a restart of the controller manager, it's possible a new pod shows up in a state that
		// is already pending deletion. Prevent the pod from being a creation observation.
		r.deletePod(pod)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		set := r.resolveControllerRef(pod.Namespace, controllerRef)
		if set == nil {
			return
		}
		klog.V(4).Infof("Pod %s created, labels: %+v", pod.Name, pod.Labels)
		r.enqueueStatefulSet(set)
		return
	}

	// Otherwise, it's an orphan. Get a list of all matching controllers and sync
	// them to see if anyone wants to adopt it.
	sets := r.getStatefulSetsForPod(pod)
	if len(sets) == 0 {
		return
	}
	klog.V(4).Infof("Orphan Pod %s created, labels: %+v", pod.Name, pod.Labels)
	for _, set := range sets {
		r.enqueueStatefulSet(set)
	}
}

// updatePod adds the statefulset for the current and old pods to the sync queue.
func (r *StatefulSetReconciler) updatePod(old, cur interface{}) {
	curPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		// In the event of a re-list we may receive update events for all known pods.
		// Two different versions of the same pod will always have different RVs.
		return
	}

	labelChanged := !reflect.DeepEqual(curPod.Labels, oldPod.Labels)

	curControllerRef := metav1.GetControllerOf(curPod)
	oldControllerRef := metav1.GetControllerOf(oldPod)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if set := r.resolveControllerRef(oldPod.Namespace, oldControllerRef); set != nil {
			r.enqueueStatefulSet(set)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		set := r.resolveControllerRef(curPod.Namespace, curControllerRef)
		if set == nil {
			return
		}
		klog.V(4).Infof("Pod %s updated, objectMeta %+v -> %+v.", curPod.Name, oldPod.ObjectMeta, curPod.ObjectMeta)
		r.enqueueStatefulSet(set)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	if labelChanged || controllerRefChanged {
		sets := r.getStatefulSetsForPod(curPod)
		if len(sets) == 0 {
			return
		}
		klog.V(4).Infof("Orphan Pod %s updated, objectMeta %+v -> %+v.", curPod.Name, oldPod.ObjectMeta, curPod.ObjectMeta)
		for _, set := range sets {
			r.enqueueStatefulSet(set)
		}
	}
}

// deletePod enqueues the statefulset for the pod accounting for deletion tombstones.
func (r *StatefulSetReconciler) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)

	// When a delete is dropped, the relist will notice a pod in the store not
	// in the list, leading to the insertion of a tombstone object which contains
	// the deleted key/value. Note that this value might be stale.
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a pod %+v", obj))
			return
		}
	}

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	set := r.resolveControllerRef(pod.Namespace, controllerRef)
	if set == nil {
		return
	}
	klog.V(4).Infof("Pod %s/%s deleted through %v.", pod.Namespace, pod.Name, utilruntime.GetCaller())
	r.enqueueStatefulSet(set)
}

// getPodsForStatefulSet returns the Pods that a given StatefulSet should manage.
// It also reconciles ControllerRef by adopting/orphaning.
//
// NOTE: Returned Pods are pointers to objects from the cache.
//       If you need to modify one, you need to copy it first.
func (r *StatefulSetReconciler) getPodsForStatefulSet(set *nuwav1.StatefulSet, selector labels.Selector) ([]*corev1.Pod, error) {
	// List all pods to include the pods that don't match the selector anymore but
	// has a ControllerRef pointing to this StatefulSet.
	//pods, err := r.podLister.Pods(set.Namespace).List(labels.Everything())
	podlist := &corev1.PodList{}
	err := r.Client.List(context.TODO(), podlist, client.InNamespace(set.Namespace), client.MatchingLabelsSelector{Selector: labels.Everything()})
	if err != nil {
		return nil, err
	}

	filter := func(pod *corev1.Pod) bool {
		// Only claim if it matches our StatefulSet name. Otherwise release/ignore.
		return isMemberOf(set, pod)
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := controller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		//fresh, err := ssc.kubeClient.AppsV1().StatefulSets(set.Namespace).Get(set.Name, metav1.GetOptions{})
		objKey := types.NamespacedName{Namespace: set.Namespace, Name: set.Name}
		fresh := &nuwav1.StatefulSet{}
		err := r.Client.Get(context.TODO(), objKey, fresh)
		if err != nil {
			return nil, err
		}
		if fresh.UID != set.UID {
			return nil, fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return fresh, nil
	})

	cm := controller.NewPodControllerRefManager(r.podControl, set, selector, controllerKind, canAdoptFunc)
	pods := make([]*corev1.Pod, 0)
	for i := range podlist.Items {
		pods = append(pods, &podlist.Items[i])
	}
	return cm.ClaimPods(pods, filter)
}

// adoptOrphanRevisions adopts any orphaned ControllerRevisions matched by set's Selector.
func (r *StatefulSetReconciler) adoptOrphanRevisions(set *nuwav1.StatefulSet) error {
	revisions, err := r.control.ListRevisions(set)
	if err != nil {
		return err
	}
	hasOrphans := false
	for i := range revisions {
		if metav1.GetControllerOf(revisions[i]) == nil {
			hasOrphans = true
			break
		}
	}
	if hasOrphans {
		objKey := types.NamespacedName{Namespace: set.Namespace, Name: set.Name}
		fresh := &nuwav1.StatefulSet{}
		err := r.Client.Get(context.TODO(), objKey, fresh)
		if err != nil {
			return err
		}
		if fresh.UID != set.UID {
			return fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return r.control.AdoptOrphanRevisions(set, revisions)
	}
	return nil
}

// getStatefulSetsForPod returns a list of StatefulSets that potentially match
// a given pod.
func (r *StatefulSetReconciler) getStatefulSetsForPod(pod *corev1.Pod) []*nuwav1.StatefulSet {
	sets, err := r.GetPodStatefulSets(pod)
	if err != nil {
		return nil
	}
	// More than one set is selecting the same Pod
	if len(sets) > 1 {
		// ControllerRef will ensure we don't do anything crazy, but more than one
		// item in this list nevertheless constitutes user error.
		utilruntime.HandleError(
			fmt.Errorf(
				"user error: more than one StatefulSet is selecting pods with labels: %+v",
				pod.Labels))
	}
	return sets
}

func (r *StatefulSetReconciler) GetPodStatefulSets(pod *corev1.Pod) ([]*nuwav1.StatefulSet, error) {
	var selector labels.Selector
	var ps *nuwav1.StatefulSet

	if len(pod.Labels) == 0 {
		return nil, fmt.Errorf("no StatefulSets found for pod %v because it has no labels", pod.Name)
	}

	//list, err := s.StatefulSets(pod.Namespace).List(labels.Everything())
	sts := &nuwav1.StatefulSetList{}
	err := r.Client.List(context.TODO(), sts, client.InNamespace(pod.Namespace), client.MatchingLabelsSelector{Selector: labels.Everything()})
	if err != nil {
		return nil, err
	}

	var psList []*nuwav1.StatefulSet
	for i := range sts.Items {
		ps = &sts.Items[i]
		if ps.Namespace != pod.Namespace {
			continue
		}
		selector, err = metav1.LabelSelectorAsSelector(ps.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector: %v", err)
		}
		// If a StatefulSet with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		psList = append(psList, ps)
	}

	if len(psList) == 0 {
		return nil, fmt.Errorf("could not find StatefulSet for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}

	return psList, nil
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (r *StatefulSetReconciler) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *nuwav1.StatefulSet {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	objKey := types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}
	//set, err := r.setLister.StatefulSets(namespace).Get(controllerRef.Name)
	set := &nuwav1.StatefulSet{}
	err := r.Client.Get(context.TODO(), objKey, set)
	if err != nil {
		return nil
	}
	if set.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return set
}

// enqueueStatefulSet enqueues the given statefulset in the work queue.
func (r *StatefulSetReconciler) enqueueStatefulSet(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %+v: %v", obj, err))
		return
	}
	r.queue.Add(key)
}

// processNextWorkItem dequeues items, processes them, and marks them done. It enforces that the syncHandler is never
// invoked concurrently with the same key.
func (r *StatefulSetReconciler) processNextWorkItem() bool {
	key, quit := r.queue.Get()
	if quit {
		return false
	}
	defer r.queue.Done(key)
	if err := r.sync(key.(string)); err != nil {
		utilruntime.HandleError(fmt.Errorf("Error syncing StatefulSet %v, requeuing: %v", key.(string), err))
		r.queue.AddRateLimited(key)
	} else {
		r.queue.Forget(key)
	}
	return true
}

// worker runs a worker goroutine that invokes processNextWorkItem until the controller's queue is closed
func (r *StatefulSetReconciler) worker() {
	for r.processNextWorkItem() {
	}
}

// sync syncs the given statefulset.
func (r *StatefulSetReconciler) sync(key string) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing statefulset %q (%v)", key, time.Since(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	objKey := types.NamespacedName{Namespace: namespace, Name: name}
	//set, err := r.setLister.StatefulSets(namespace).Get(name)
	set := &nuwav1.StatefulSet{}
	err = r.Client.Get(context.TODO(), objKey, set)
	if apierrors.IsNotFound(err) {
		klog.Infof("StatefulSet has been deleted %v", key)
		return nil
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to retrieve StatefulSet %v from store: %v", key, err))
		return err
	}

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error converting StatefulSet %v selector: %v", key, err))
		// This is a non-transient error, so don't retry.
		return nil
	}

	if err := r.adoptOrphanRevisions(set); err != nil {
		return err
	}

	pods, err := r.getPodsForStatefulSet(set, selector)
	if err != nil {
		return err
	}

	return r.syncStatefulSet(set, pods)
}

// syncStatefulSet syncs a tuple of (statefulset, []*v1.Pod).
func (r *StatefulSetReconciler) syncStatefulSet(set *nuwav1.StatefulSet, pods []*corev1.Pod) error {
	klog.V(4).Infof("Syncing StatefulSet %v/%v with %d pods", set.Namespace, set.Name, len(pods))
	// TODO: investigate where we mutate the set during the update as it is not obvious.
	if err := r.control.UpdateStatefulSet(set.DeepCopy(), pods); err != nil {
		return err
	}
	klog.V(4).Infof("Successfully synced StatefulSet %s/%s successful", set.Namespace, set.Name)
	return nil
}

func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	podInformer, err := mgr.GetCache().GetInformer(&corev1.Pod{})
	if err != nil {
		return err
	}
	podInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			// lookup the statefulset and enqueue
			AddFunc: r.addPod,
			// lookup current and old statefulset if labels changed
			UpdateFunc: r.updatePod,
			// lookup statefulset accounting for deletion tombstones
			DeleteFunc: r.deletePod,
		},
	)
	//r.control = r
	//r.podControl = r

	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.StatefulSet{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.PersistentVolume{}).
		Complete(r)
}
