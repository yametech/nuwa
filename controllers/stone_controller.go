/*
Copyright 2019 yametech.

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
)

// StoneReconciler reconciles a Stone object
type StoneReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=statefulsets/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones/status,verbs=get;update;patch
func (r *StoneReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logf := r.Log.WithValues("stone", req.NamespacedName)
	_, _ = ctx, logf

	ste := &nuwav1.Stone{}
	if err := r.Client.Get(ctx, req.NamespacedName, ste); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.cleanOldStatefulSet(ctx, logf, ste); err != nil {
		return ctrl.Result{}, err
	}

	statefulSet, err := r.getStatefulSet(ctx, logf, ste)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateStatefulSet(ctx, logf, statefulSet); err != nil {
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

func (r *StoneReconciler) getStatefulSet(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) (*nuwav1.StatefulSet, error) {
	return nil, nil
}

func (r *StoneReconciler) createService(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {

	return nil
}

func (r *StoneReconciler) updateStatefulSet(ctx context.Context, log logr.Logger, sts *nuwav1.StatefulSet) error {

	return nil
}

func (r *StoneReconciler) cleanOldStatefulSet(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {
	if ste.Annotations == nil {
		return nil
	}

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
		sourceTop, sourceGroup, err := makeGroupCoordinator(r.Client, source)
		if err != nil {
			return err
		}
		_, _ = sourceTop, sourceGroup

		target := ste.Spec.Coordinates.DeepCopy()
		targetTop, targetGroup, err := makeGroupCoordinator(r.Client, target)
		if err != nil {
			return err
		}
		_, _ = targetTop, targetGroup
		
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
