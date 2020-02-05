/*
Copyright 2019 yametech Authors.

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
	"encoding/json"
	"github.com/go-logr/logr"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
)

// StoneReconciler reconciles a Stone object
type StoneReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// stone named rule
//
// stone_name = "$user_define"
// group_name = "$index"
// stateful_name = $stone_name + $group_name

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones/status,verbs=get;update;patch
func (r *StoneReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logf := r.Log.WithValues("stone", req.NamespacedName)

	ste := &nuwav1.Stone{}
	if err := r.Client.Get(ctx, req.NamespacedName, ste); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.syncStatefulSet(ctx, logf, ste); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createService(ctx, logf, ste); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateStone(ctx, logf, ste); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *StoneReconciler) getStatefulSet(ctx context.Context, log logr.Logger, last nuwav1.Coordinates, ste *nuwav1.Stone) ([]*nuwav1.StatefulSet, error) {
	var statefulSetGroup map[int]nuwav1.Coordinates
	var numberOfGroup int
	var err error

	if last != nil {
		srouce := ste.Spec.Coordinates.DeepCopy()
		statefulSetGroup, numberOfGroup, err = splitGroupCoordinates(nuwav1.Difference(srouce, last))
		if err != nil {
			return nil, err
		}
	}

	_ = statefulSetGroup
	stsPointerSlice := make([]*nuwav1.StatefulSet, 0)

	for i := 1; i <= numberOfGroup; i++ {
		statefulSetName := statefulSetName(ste, i)
		sts := &nuwav1.StatefulSet{}
		key := client.ObjectKey{Name: statefulSetName, Namespace: ste.Namespace}
		if err := r.Client.Get(ctx, key, sts); err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
			log.Info(
				"statefulSet not found",
				"namespace",
				ste.Namespace,
				"statefulSetName",
				statefulSetName,
			)

			// default is the number of machines per group
			var size int32
			if ste.Spec.Strategy == nuwav1.Alpha {
				size = 1
			} else if ste.Spec.Strategy == nuwav1.Beta {
				size = 1
			} else if ste.Spec.Strategy == nuwav1.Release {
				//
			}
			// structure new statefulset
			labels := map[string]string{"app": statefulSetName}
			selector := &metav1.LabelSelector{MatchLabels: labels}
			sts = &nuwav1.StatefulSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "nuwa.nip.io/v1", Kind: "StatefulSet"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            statefulSetName,
					Namespace:       ste.Namespace,
					OwnerReferences: ownerReference(ste, "Stone"),
				},

				Spec: nuwav1.StatefulSetSpec{
					Replicas: &size,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec:       ste.Spec.Template.Spec,
					},
					Selector: selector,
				},
			}
		}
		stsPointerSlice = append(stsPointerSlice, sts)
	}

	return stsPointerSlice, nil
}

func (r *StoneReconciler) createService(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {

	return nil
}

func (r *StoneReconciler) updateStatefulSet(ctx context.Context, log logr.Logger, sts *nuwav1.StatefulSet) error {

	return nil
}

func (r *StoneReconciler) syncStatefulSet(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var last nuwav1.Coordinates
		var tmpSteSpec nuwav1.StoneSpec
		str, ok := ste.Annotations["spec"]
		if ok {
			if err := json.Unmarshal([]byte(str), &tmpSteSpec); err != nil {
				return err
			}
			last = tmpSteSpec.Coordinates.DeepCopy()
		}

		statefulSets, err := r.getStatefulSet(ctx, log, last, ste)
		if err != nil {
			return err
		}

		for i := range statefulSets {
			if err := r.Client.Update(ctx, statefulSets[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *StoneReconciler) updateStone(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {
	return nil
}

func (r *StoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Stone{}).
		Owns(&nuwav1.StatefulSet{}).
		Complete(r)
}

