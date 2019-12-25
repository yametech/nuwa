/*
Copyright 2019 The yametech Authors.
Copyright 2019 The Kruise Authors.
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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

// StatefulSetReconciler reconciles a StatefulSet object
type StatefulSetReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	// control returns an interface capable of syncing a stateful set.
	// Abstracted out for testing.
	control ControlInterface
	// podControl is used for patching pods.
	podControl kubecontroller.PodControlInterface
	// podLister is able to list/get pods from a shared informer's store
	//podLister corelisters.PodLister
}

func (ssc *StatefulSetReconciler) checkSetDefaultVariable(setSpec *nuwav1.StatefulSetSpec) error {
	if setSpec.Replicas == nil {
		return fmt.Errorf("Replicas not define")
	}
	if setSpec.UpdateStrategy == nil {
		return fmt.Errorf("UpdateStrategy not define")
	}
	if setSpec.UpdateStrategy.RollingUpdate == nil {
		return fmt.Errorf("UpdateStrategy.RollingUpdate not define")
	}
	if setSpec.UpdateStrategy.RollingUpdate.Partition == nil {
		var partition int32 = 1
		setSpec.UpdateStrategy.RollingUpdate.Partition = &partition
		return fmt.Errorf("UpdateStrategy.RollingUpdate.Partition not define")
	}
	return nil
}

// Reconcile reads that state of the cluster for a StatefulSet object and makes changes based on the state read
// and what is in the StatefulSet.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Pods
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets/status,verbs=get;update;patch
func (ssc *StatefulSetReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	key := req.NamespacedName.String()
	ctx := context.Background()

	startTime := time.Now()
	defer func() {
		ssc.Log.Info("Finished syncing statefulset", "key", key, "time_since", time.Since(startTime))
	}()

	set := &nuwav1.StatefulSet{}
	err := ssc.Client.Get(ctx, req.NamespacedName, set)
	if errors.IsNotFound(err) {
		ssc.Log.Info("StatefulSet has been deleted", "key", key)
		updateExpectations.DeleteExpectations(key)
		return reconcile.Result{}, nil
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to retrieve StatefulSet %v from store: %v", key, err))
		return reconcile.Result{}, err
	}

	if err := ssc.checkSetDefaultVariable(&set.Spec); err != nil {
		utilruntime.HandleError(fmt.Errorf("check StatefulSet %v error: %v", key, err))
	}

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error converting StatefulSet %v selector: %v", key, err))
		// This is a non-transient error, so don't retry.
		return reconcile.Result{}, nil
	}

	if err := ssc.adoptOrphanRevisions(set); err != nil {
		return reconcile.Result{}, err
	}

	pods, err := ssc.getPodsForStatefulSet(set, selector)
	if err != nil {
		return reconcile.Result{}, nil
	}

	if err := ssc.syncStatefulSet(set, pods); err != nil {
		ssc.Log.Info("sync statfulset error", "error", err)
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

// adoptOrphanRevisions adopts any orphaned ControllerRevisions matched by set's Selector.
func (ssc *StatefulSetReconciler) adoptOrphanRevisions(set *nuwav1.StatefulSet) error {
	revisions, err := ssc.control.ListRevisions(set)
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
		//fresh, err := ssc.kruiseClient.AppsV1alpha1().StatefulSets(set.Namespace).Get(set.Name, metav1.GetOptions{})
		fresh := &nuwav1.StatefulSet{}
		ctx := context.Background()
		objKey := types.NamespacedName{Namespace: set.Namespace, Name: set.Name}
		err := ssc.Client.Get(ctx, objKey, fresh)
		if err != nil {
			return err
		}
		if fresh.UID != set.UID {
			return fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return ssc.control.AdoptOrphanRevisions(set, revisions)
	}
	return nil
}

// getPodsForStatefulSet returns the Pods that a given StatefulSet should manage.
// It also reconciles ControllerRef by adopting/orphaning.
//
// NOTE: Returned Pods are pointers to objects from the cache.
//       If you need to modify one, you need to copy it first.
func (ssc *StatefulSetReconciler) getPodsForStatefulSet(set *nuwav1.StatefulSet, selector labels.Selector) ([]*v1.Pod, error) {
	// List all pods to include the pods that don't match the selector anymore but
	// has a ControllerRef pointing to this StatefulSet.
	//pods, err := ssc.podLister.Pods(set.Namespace).List(labels.Everything())
	podList := &v1.PodList{}
	err := ssc.Client.List(context.Background(), podList, client.InNamespace(set.Namespace))
	if err != nil {
		return nil, err
	}

	filter := func(pod *v1.Pod) bool {
		// Only claim if it matches our StatefulSet name. Otherwise release/ignore.
		return isMemberOf(set, pod)
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		//fresh, err := ssc.kruiseClient.AppsV1alpha1().StatefulSets(set.Namespace).Get(set.Name, metav1.GetOptions{})
		fresh := &nuwav1.StatefulSet{}
		ctx := context.Background()
		objKey := client.ObjectKey{Namespace: set.Namespace, Name: set.Name}
		err := ssc.Client.Get(ctx, objKey, fresh)
		if err != nil {
			return nil, err
		}
		if fresh.UID != set.UID {
			return nil, fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return fresh, nil
	})

	cm := kubecontroller.NewPodControllerRefManager(ssc.podControl, set, selector, controllerKind, canAdoptFunc)
	pods := make([]*v1.Pod, 0, len(podList.Items))
	for i := range pods {
		pods[i] = &podList.Items[i]
	}
	return cm.ClaimPods(pods, filter)
}

// syncStatefulSet syncs a tuple of (statefulset, []*v1.Pod).
func (ssc *StatefulSetReconciler) syncStatefulSet(set *nuwav1.StatefulSet, pods []*v1.Pod) error {
	ssc.Log.Info("Syncing StatefulSet", "namespace", set.Namespace, "name", set.Name, "pods", len(pods))
	// TODO: investigate where we mutate the set during the update as it is not obvious.
	if err := ssc.control.UpdateStatefulSet(set.DeepCopy(), pods); err != nil {
		return err
	}
	return nil
}

var jobOwnerKey = ".metadata.controller"
var apiGVStr = nuwav1.GroupVersion.String()

func (ssc *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	patchCodec = serializer.NewCodecFactory(ssc.Scheme).LegacyCodec(nuwav1.GroupVersion)
	recorder := mgr.GetEventRecorderFor("statefulset-controller")
	statefulsetPodControl := NewRealStatefulPodControl(ssc.Client, recorder)
	ssc.control = NewDefaultStatefulSetControl(
		statefulsetPodControl,
		NewRealStatefulSetStatusUpdater(ssc.Client),
		&realHistory{ssc.Client},
		recorder,
		ssc.Log,
	)
	ssc.podControl = &RealPodControl{Client: ssc.Client, Recorder: recorder}

	if err := mgr.GetFieldIndexer().IndexField(&nuwav1.StatefulSet{}, jobOwnerKey, func(rawObj runtime.Object) []string {
		job := rawObj.(*nuwav1.StatefulSet)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != apiGVStr || owner.Kind != "StatefulSet" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.StatefulSet{}).
		Watches(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}).
		Complete(ssc)
}
