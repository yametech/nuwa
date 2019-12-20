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
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"
	"math"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
	"time"
)

var controllerKind = nuwav1.GroupVersion.WithKind("StatefulSet")
var patchCodec runtime.Codec

// StatefulSetReconciler reconciles a StatefulSet object
type StatefulSetReconciler struct {
	// Default
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	// podControl is used for patching pods.
	podControl controller.PodControlInterface
	// controllerHistory
	controllerHistory history.Interface
	// statusUpdater
	statusUpdater StatefulSetStatusUpdaterInterface
}

func (r *StatefulSetReconciler) CreateStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error {
	// Create the Pod's PVCs prior to creating the Pod
	if err := r.createPersistentVolumeClaims(set, pod); err != nil {
		return err
	}
	// If we created the PVCs attempt to create the Pod
	pod.Namespace = set.Namespace
	err := r.Client.Create(context.TODO(), pod)
	// sink already exists errors
	if apierrors.IsAlreadyExists(err) {
		return err
	}
	return err
}

func (r *StatefulSetReconciler) UpdateStatefulPod(set *nuwav1.StatefulSet, pod *corev1.Pod) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
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
				return err
			}
		}
		// if the Pod is not dirty, do nothing
		if consistent {
			return nil
		}
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
		case err != nil:
			errs = append(errs, fmt.Errorf("failed to retrieve PVC %s: %s", claim.Name, err))
		}
		// TODO: Check resource requirements and accessmodes, update if necessary
	}
	return errors.NewAggregate(errs)
}

// addPod adds the statefulset for the pod to the sync queue
func (r *StatefulSetReconciler) addPod(obj interface{}) {
	pod := obj.(*corev1.Pod)
	logf := r.Log.WithName("StatefulSetReconciler")
	logf.Info("addPod", "namespace", pod.Namespace, "name", pod.Name)

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
		r.Log.V(4).Info("Pod created", "name", pod.Name, "labels", pod.Labels)
		//r.enqueueStatefulSet(set)
		return
	}

	// Otherwise, it's an orphan. Get a list of all matching controllers and sync
	// them to see if anyone wants to adopt it.
	sets := r.getStatefulSetsForPod(pod)
	if len(sets) == 0 {
		return
	}
	klog.V(4).Infof("Orphan Pod %s created, labels: %+v", pod.Name, pod.Labels)
	//for _, set := range sets {
	//	r.enqueueStatefulSet(set)
	//}
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
			//r.enqueueStatefulSet(set)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		set := r.resolveControllerRef(curPod.Namespace, curControllerRef)
		if set == nil {
			return
		}
		//klog.V(4).Infof("Pod %s updated, objectMeta %+v -> %+v.", curPod.Name, oldPod.ObjectMeta, curPod.ObjectMeta)
		//r.enqueueStatefulSet(set)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	if labelChanged || controllerRefChanged {
		sets := r.getStatefulSetsForPod(curPod)
		if len(sets) == 0 {
			return
		}
		//klog.V(4).Infof("Orphan Pod %s updated, objectMeta %+v -> %+v.", curPod.Name, oldPod.ObjectMeta, curPod.ObjectMeta)
		//for _, set := range sets {
		//	r.enqueueStatefulSet(set)
		//}
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
	//klog.V(4).Infof("Pod %s/%s deleted through %v.", pod.Namespace, pod.Name, utilruntime.GetCaller())
	//r.enqueueStatefulSet(set)
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
	ctx := context.Background()
	podList := &corev1.PodList{}
	err := r.Client.List(ctx, podList, client.InNamespace(set.Namespace), client.MatchingLabelsSelector{Selector: labels.Everything()})
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
		objKey := types.NamespacedName{Namespace: set.Namespace, Name: set.Name}
		fresh := &nuwav1.StatefulSet{}
		err := r.Client.Get(ctx, objKey, fresh)
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
	for i := range podList.Items {
		pods = append(pods, &podList.Items[i])
	}

	return cm.ClaimPods(pods, filter)
}

// adoptOrphanRevisions adopts any orphaned ControllerRevisions matched by set's Selector.
func (r *StatefulSetReconciler) adoptOrphanRevisions(set *nuwav1.StatefulSet) error {
	revisions, err := r.ListRevisions(set)
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
		return r.AdoptOrphanRevisions(set, revisions)
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
	err := r.Client.List(context.TODO(), sts,
		client.InNamespace(pod.Namespace),
		client.MatchingLabelsSelector{Selector: labels.Everything()},
	)
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

// sync syncs the given statefulset.
func (r *StatefulSetReconciler) sync(key string) error {
	startTime := time.Now()
	defer func() {
		r.Log.V(4).Info("Finished syncing statefulset", "key", key, "startTime", time.Since(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	objKey := types.NamespacedName{Namespace: namespace, Name: name}
	set := &nuwav1.StatefulSet{}
	ctx := context.Background()
	err = r.Client.Get(ctx, objKey, set)
	if apierrors.IsNotFound(err) {
		r.Log.V(4).Info("StatefulSet has been deleted", "key", key)
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
	// TODO: investigate where we mutate the set during the update as it is not obvious.
	if err := r.UpdateStatefulSet(set.DeepCopy(), pods); err != nil {
		return err
	}
	return nil
}

// UpdateStatefulSet executes the core logic loop for a stateful set, applying the predictable and
// consistent monotonic update strategy by default - scale up proceeds in ordinal order, no new pod
// is created while any pod is unhealthy, and pods are terminated in descending order. The burst
// strategy allows these constraints to be relaxed - pods will be created and deleted eagerly and
// in no particular order. Clients using the burst strategy should be careful to ensure they
// understand the consistency implications of having unpredictable numbers of pods available.
func (r *StatefulSetReconciler) UpdateStatefulSet(set *nuwav1.StatefulSet, pods []*corev1.Pod) error {
	// list all revisions and sort them
	revisions, err := r.ListRevisions(set)
	if err != nil {
		return err
	}
	history.SortControllerRevisions(revisions)

	// get the current, and update revisions
	currentRevision, updateRevision, collisionCount, err := r.getStatefulSetRevisions(set, revisions)
	if err != nil {
		return err
	}

	// perform the main update function and get the status
	status, err := r.updateStatefulSet(set, currentRevision, updateRevision, collisionCount, pods)
	if err != nil {
		return err
	}

	// update the set's status
	if err := r.updateStatefulSetStatus(set, status); err != nil {
		return err
	}

	// maintain the set's revision history limit
	if err := r.truncateHistory(set, pods, revisions, currentRevision, updateRevision); err != nil {
		return err
	}

	return nil
}

func (r *StatefulSetReconciler) ListRevisions(set *nuwav1.StatefulSet) ([]*appsv1.ControllerRevision, error) {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	return r.controllerHistory.ListControllerRevisions(set, selector)
}

func (r *StatefulSetReconciler) AdoptOrphanRevisions(set *nuwav1.StatefulSet, revisions []*appsv1.ControllerRevision) error {
	for i := range revisions {
		adopted, err := r.controllerHistory.AdoptControllerRevision(set, controllerKind, revisions[i])
		if err != nil {
			return err
		}
		revisions[i] = adopted
	}
	return nil
}

// truncateHistory truncates any non-live ControllerRevisions in revisions from set's history. The UpdateRevision and
// CurrentRevision in set's Status are considered to be live. Any revisions associated with the Pods in pods are also
// considered to be live. Non-live revisions are deleted, starting with the revision with the lowest Revision, until
// only RevisionHistoryLimit revisions remain. If the returned error is nil the operation was successful. This method
// expects that revisions is sorted when supplied.
func (r *StatefulSetReconciler) truncateHistory(
	set *nuwav1.StatefulSet,
	pods []*corev1.Pod,
	revisions []*appsv1.ControllerRevision,
	current *appsv1.ControllerRevision,
	update *appsv1.ControllerRevision) error {
	history := make([]*appsv1.ControllerRevision, 0, len(revisions))
	// mark all live revisions
	live := map[string]bool{current.Name: true, update.Name: true}
	for i := range pods {
		live[getPodRevision(pods[i])] = true
	}
	// collect live revisions and historic revisions
	for i := range revisions {
		if !live[revisions[i].Name] {
			history = append(history, revisions[i])
		}
	}
	historyLen := len(history)
	historyLimit := int(*set.Spec.RevisionHistoryLimit)
	if historyLen <= historyLimit {
		return nil
	}
	// delete any non-live history to maintain the revision limit.
	history = history[:(historyLen - historyLimit)]
	for i := 0; i < len(history); i++ {
		if err := r.controllerHistory.DeleteControllerRevision(history[i]); err != nil {
			return err
		}
	}
	return nil
}

// getStatefulSetRevisions returns the current and update ControllerRevisions for set. It also
// returns a collision count that records the number of name collisions set saw when creating
// new ControllerRevisions. This count is incremented on every name collision and is used in
// building the ControllerRevision names for name collision avoidance. This method may create
// a new revision, or modify the Revision of an existing revision if an update to set is detected.
// This method expects that revisions is sorted when supplied.
func (r *StatefulSetReconciler) getStatefulSetRevisions(
	set *nuwav1.StatefulSet, revisions []*appsv1.ControllerRevision) (
	*appsv1.ControllerRevision, *appsv1.ControllerRevision, int32, error) {
	var currentRevision, updateRevision *appsv1.ControllerRevision

	revisionCount := len(revisions)
	history.SortControllerRevisions(revisions)

	// Use a local copy of set.Status.CollisionCount to avoid modifying set.Status directly.
	// This copy is returned so the value gets carried over to set.Status in updateStatefulSet.
	var collisionCount int32
	if set.Status.CollisionCount != nil {
		collisionCount = *set.Status.CollisionCount
	}

	// create a new revision from the current set
	updateRevision, err := newRevision(set, nextRevision(revisions), &collisionCount)
	if err != nil {
		return nil, nil, collisionCount, err
	}

	// find any equivalent revisions
	equalRevisions := history.FindEqualRevisions(revisions, updateRevision)
	equalCount := len(equalRevisions)

	if equalCount > 0 && history.EqualRevision(revisions[revisionCount-1], equalRevisions[equalCount-1]) {
		// if the equivalent revision is immediately prior the update revision has not changed
		updateRevision = revisions[revisionCount-1]
	} else if equalCount > 0 {
		// if the equivalent revision is not immediately prior we will roll back by incrementing the
		// Revision of the equivalent revision
		updateRevision, err = r.controllerHistory.UpdateControllerRevision(
			equalRevisions[equalCount-1],
			updateRevision.Revision)
		if err != nil {
			return nil, nil, collisionCount, err
		}
	} else {
		//if there is no equivalent revision we create a new one
		updateRevision, err = r.controllerHistory.CreateControllerRevision(set, updateRevision, &collisionCount)
		if err != nil {
			return nil, nil, collisionCount, err
		}
	}

	// attempt to find the revision that corresponds to the current revision
	for i := range revisions {
		if revisions[i].Name == set.Status.CurrentRevision {
			currentRevision = revisions[i]
			break
		}
	}

	// if the current revision is nil we initialize the history by setting it to the update revision
	if currentRevision == nil {
		currentRevision = updateRevision
	}

	return currentRevision, updateRevision, collisionCount, nil
}

// updateStatefulSet performs the update function for a StatefulSet. This method creates, updates, and deletes Pods in
// the set in order to conform the system to the target state for the set. The target state always contains
// set.Spec.Replicas Pods with a Ready Condition. If the UpdateStrategy.Type for the set is
// RollingUpdateStatefulSetStrategyType then all Pods in the set must be at set.Status.CurrentRevision.
// If the UpdateStrategy.Type for the set is OnDeleteStatefulSetStrategyType, the target state implies nothing about
// the revisions of Pods in the set. If the UpdateStrategy.Type for the set is PartitionStatefulSetStrategyType, then
// all Pods with ordinal less than UpdateStrategy.Partition.Ordinal must be at Status.CurrentRevision and all other
// Pods must be at Status.UpdateRevision. If the returned error is nil, the returned StatefulSetStatus is valid and the
// update must be recorded. If the error is not nil, the method should be retried until successful.
func (r *StatefulSetReconciler) updateStatefulSet(
	set *nuwav1.StatefulSet,
	currentRevision *appsv1.ControllerRevision,
	updateRevision *appsv1.ControllerRevision,
	collisionCount int32,
	pods []*corev1.Pod) (*nuwav1.StatefulSetStatus, error) {
	// get the current and update revisions of the set.
	currentSet, err := ApplyRevision(set, currentRevision)
	if err != nil {
		return nil, err
	}
	updateSet, err := ApplyRevision(set, updateRevision)
	if err != nil {
		return nil, err
	}

	// set the generation, and revisions in the returned status
	status := nuwav1.StatefulSetStatus{}
	status.ObservedGeneration = set.Generation
	status.CurrentRevision = currentRevision.Name
	status.UpdateRevision = updateRevision.Name
	status.CollisionCount = new(int32)
	*status.CollisionCount = collisionCount

	replicaCount := int(*set.Spec.Replicas)
	// slice that will contain all Pods such that 0 <= getOrdinal(pod) < set.Spec.Replicas
	replicas := make([]*corev1.Pod, replicaCount)
	// slice that will contain all Pods such that set.Spec.Replicas <= getOrdinal(pod)
	condemned := make([]*corev1.Pod, 0, len(pods))
	unhealthy := 0
	firstUnhealthyOrdinal := math.MaxInt32
	var firstUnhealthyPod *corev1.Pod

	// First we partition pods into two lists valid replicas and condemned Pods
	for i := range pods {
		status.Replicas++

		// count the number of running and ready replicas
		if isRunningAndReady(pods[i]) {
			status.ReadyReplicas++
		}

		// count the number of current and update replicas
		if isCreated(pods[i]) && !isTerminating(pods[i]) {
			if getPodRevision(pods[i]) == currentRevision.Name {
				status.CurrentReplicas++
			}
			if getPodRevision(pods[i]) == updateRevision.Name {
				status.UpdatedReplicas++
			}
		}

		if ord := getOrdinal(pods[i]); 0 <= ord && ord < replicaCount {
			// if the ordinal of the pod is within the range of the current number of replicas,
			// insert it at the indirection of its ordinal
			replicas[ord] = pods[i]

		} else if ord >= replicaCount {
			// if the ordinal is greater than the number of replicas add it to the condemned list
			condemned = append(condemned, pods[i])
		}
		// If the ordinal could not be parsed (ord < 0), ignore the Pod.
	}

	// for any empty indices in the sequence [0,set.Spec.Replicas) create a new Pod at the correct revision
	for ord := 0; ord < replicaCount; ord++ {
		if replicas[ord] == nil {
			replicas[ord] = newVersionedStatefulSetPod(
				currentSet,
				updateSet,
				currentRevision.Name,
				updateRevision.Name, ord)
		}
	}

	// sort the condemned Pods by their ordinals
	sort.Sort(ascendingOrdinal(condemned))

	// find the first unhealthy Pod
	for i := range replicas {
		if !isHealthy(replicas[i]) {
			unhealthy++
			if ord := getOrdinal(replicas[i]); ord < firstUnhealthyOrdinal {
				firstUnhealthyOrdinal = ord
				firstUnhealthyPod = replicas[i]
			}
		}
	}

	for i := range condemned {
		if !isHealthy(condemned[i]) {
			unhealthy++
			if ord := getOrdinal(condemned[i]); ord < firstUnhealthyOrdinal {
				firstUnhealthyOrdinal = ord
				firstUnhealthyPod = condemned[i]
			}
		}
	}

	if unhealthy > 0 {
	}

	// If the StatefulSet is being deleted, don't do anything other than updating
	// status.
	if set.DeletionTimestamp != nil {
		return &status, nil
	}

	monotonic := !allowsBurst(set)

	// Examine each replica with respect to its ordinal
	for i := range replicas {
		// delete and recreate failed pods
		if isFailed(replicas[i]) {
			//r.recorder.Eventf(set, corev1.EventTypeWarning, "RecreatingFailedPod",
			//	"StatefulSet %s/%s is recreating failed Pod %s",
			//	set.Namespace,
			//	set.Name,
			//	replicas[i].Name)
			if err := r.DeleteStatefulPod(set, replicas[i]); err != nil {
				return &status, err
			}
			if getPodRevision(replicas[i]) == currentRevision.Name {
				status.CurrentReplicas--
			}
			if getPodRevision(replicas[i]) == updateRevision.Name {
				status.UpdatedReplicas--
			}
			status.Replicas--
			replicas[i] = newVersionedStatefulSetPod(
				currentSet,
				updateSet,
				currentRevision.Name,
				updateRevision.Name,
				i)
		}
		// If we find a Pod that has not been created we create the Pod
		if !isCreated(replicas[i]) {
			if err := r.CreateStatefulPod(set, replicas[i]); err != nil {
				return &status, err
			}
			status.Replicas++
			if getPodRevision(replicas[i]) == currentRevision.Name {
				status.CurrentReplicas++
			}
			if getPodRevision(replicas[i]) == updateRevision.Name {
				status.UpdatedReplicas++
			}

			// if the set does not allow bursting, return immediately
			if monotonic {
				return &status, nil
			}
			// pod created, no more work possible for this round
			continue
		}
		// If we find a Pod that is currently terminating, we must wait until graceful deletion
		// completes before we continue to make progress.
		if isTerminating(replicas[i]) && monotonic {
			return &status, nil
		}
		// If we have a Pod that has been created but is not running and ready we can not make progress.
		// We must ensure that all for each Pod, when we create it, all of its predecessors, with respect to its
		// ordinal, are Running and Ready.
		if !isRunningAndReady(replicas[i]) && monotonic {
			return &status, nil
		}
		// Enforce the StatefulSet invariants
		if identityMatches(set, replicas[i]) && storageMatches(set, replicas[i]) {
			continue
		}
		// Make a deep copy so we don't mutate the shared cache
		replica := replicas[i].DeepCopy()
		if err := r.UpdateStatefulPod(updateSet, replica); err != nil {
			return &status, err
		}
	}

	// At this point, all of the current Replicas are Running and Ready, we can consider termination.
	// We will wait for all predecessors to be Running and Ready prior to attempting a deletion.
	// We will terminate Pods in a monotonically decreasing order over [len(pods),set.Spec.Replicas).
	// Note that we do not resurrect Pods in this interval. Also note that scaling will take precedence over
	// updates.
	for target := len(condemned) - 1; target >= 0; target-- {
		// wait for terminating pods to expire
		if isTerminating(condemned[target]) {
			// block if we are in monotonic mode
			if monotonic {
				return &status, nil
			}
			continue
		}
		// if we are in monotonic mode and the condemned target is not the first unhealthy Pod block
		if !isRunningAndReady(condemned[target]) && monotonic && condemned[target] != firstUnhealthyPod {
			return &status, nil
		}

		if err := r.DeleteStatefulPod(set, condemned[target]); err != nil {
			return &status, err
		}
		if getPodRevision(condemned[target]) == currentRevision.Name {
			status.CurrentReplicas--
		}
		if getPodRevision(condemned[target]) == updateRevision.Name {
			status.UpdatedReplicas--
		}
		if monotonic {
			return &status, nil
		}
	}

	// for the OnDelete strategy we short circuit. Pods will be updated when they are manually deleted.
	if set.Spec.UpdateStrategy.Type == nuwav1.OnDeleteStatefulSetStrategyType {
		return &status, nil
	}

	// we compute the minimum ordinal of the target sequence for a destructive update based on the strategy.
	updateMin := 0
	if set.Spec.UpdateStrategy.RollingUpdate != nil {
		updateMin = int(*set.Spec.UpdateStrategy.RollingUpdate.Partition)
	}
	// we terminate the Pod with the largest ordinal that does not match the update revision.
	for target := len(replicas) - 1; target >= updateMin; target-- {

		// delete the Pod if it is not already terminating and does not match the update revision.
		if getPodRevision(replicas[target]) != updateRevision.Name && !isTerminating(replicas[target]) {
			err := r.DeleteStatefulPod(set, replicas[target])
			status.CurrentReplicas--
			return &status, err
		}

		// wait for unhealthy Pods on update
		if !isHealthy(replicas[target]) {
			return &status, nil
		}

	}
	return &status, nil
}

// updateStatefulSetStatus updates set's Status to be equal to status. If status indicates a complete update, it is
// mutated to indicate completion. If status is semantically equivalent to set's Status no update is performed. If the
// returned error is nil, the update is successful.
func (r *StatefulSetReconciler) updateStatefulSetStatus(set *nuwav1.StatefulSet, status *nuwav1.StatefulSetStatus) error {
	// complete any in progress rolling update if necessary
	completeRollingUpdate(set, status)

	// if the status is not inconsistent do not perform an update
	if !inconsistentStatus(set, status) {
		return nil
	}
	// copy set and update its status
	set = set.DeepCopy()
	if err := r.statusUpdater.UpdateStatefulSetStatus(set, status); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets/status,verbs=get;update;patch
func (r *StatefulSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	sts := &nuwav1.StatefulSet{}
	if err := r.Client.Get(ctx, req.NamespacedName, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	key := req.NamespacedName.String()
	if err := r.sync(key); err != nil {
		utilruntime.HandleError(fmt.Errorf("Error syncing StatefulSet %v, requeuing: %v", req.NamespacedName.String(), err))
	}

	return ctrl.Result{}, nil
}

func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.podControl = &RealPodControl{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("statefulset-controller"),
	}
	r.statusUpdater = NewRealStatefulSetStatusUpdater(mgr.GetClient())
	r.controllerHistory = &realHistory{mgr.GetClient()}
	patchCodec = serializer.NewCodecFactory(r.Scheme).LegacyCodec(nuwav1.GroupVersion)

	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.StatefulSet{}).
		Watches(&source.Kind{Type: &corev1.Pod{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &nuwav1.StatefulSet{},
			}).
		Complete(r)
}
