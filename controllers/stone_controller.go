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
	"k8s.io/apimachinery/pkg/api/errors"
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

func (r *StoneReconciler) getStatefulSet(ctx context.Context, log logr.Logger, ste *nuwav1.Stone, statefulSetName string) (*nuwav1.StatefulSet, error) {
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
		// structure
		sts.Namespace = ste.Namespace
		sts.Name = statefulSetName
		sts.Spec.Template = ste.Spec.Template
	}
	return sts, nil
}

func (r *StoneReconciler) createService(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {

	return nil
}

func (r *StoneReconciler) updateStatefulSet(ctx context.Context, log logr.Logger, sts *nuwav1.StatefulSet) error {

	return nil
}

func (r *StoneReconciler) syncStatefulSet(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var tmpSteSpec nuwav1.StoneSpec
		str, ok := ste.Annotations["spec"]
		if !ok {
			return nil
		}
		if err := json.Unmarshal([]byte(str), &tmpSteSpec); err != nil {
			return err
		}

		source := tmpSteSpec.Coordinates.DeepCopy()
		target := ste.Spec.Coordinates.DeepCopy()
		needDeleted := nuwav1.Difference(source, target)

		needChanged, _, err := splitGroupCoordinates(needDeleted)
		if err != nil {
			return err
		}

		for index, _ := range needChanged {
			statefulSetName := statefulSetName(ste, index)
			statefulSetSpec, err := r.getStatefulSet(ctx, log, ste, statefulSetName)
			if err != nil {
				return err
			}
			_ = statefulSetSpec
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

func splitGroupCoordinates(coordinates nuwav1.Coordinates) (map[int]nuwav1.Coordinates, int, error) {
	sort.Sort(&coordinates)
	result := make(map[int]nuwav1.Coordinates)
	group := 0
	curZone := ""
	for i := range coordinates {
		if curZone != coordinates[i].Zone {
			group++
		}
		if _, ok := result[group]; !ok {
			result[group] = make(nuwav1.Coordinates, 0)
		}
		result[group] = append(result[group], coordinates[i])

		curZone = coordinates[i].Zone
	}
	return result, group, nil
}
