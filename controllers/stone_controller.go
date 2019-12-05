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
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nuwav1 "github.com/yametech/nuwa/api/v1"
)

// StoneReconciler reconciles a Stone object
type StoneReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones/status,verbs=get;update;patch

func (r *StoneReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logf := r.Log.WithValues("stone", req.NamespacedName)

	instance := &nuwav1.Stone{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logf.Info("Update statefulset")
	if err := r.updateStatefulSet(ctx, req, instance); err != nil {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *StoneReconciler) updateStatefulSet(ctx context.Context, req ctrl.Request, instance *nuwav1.Stone) error {
	sts := &appsv1.StatefulSet{}
	if err := r.Client.Get(ctx, req.NamespacedName, sts); err != nil {
		if errors.IsNotFound(err) {
			// create it

		} else {
			return err
		}
	}
	return nil
}

func (r *StoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Stone{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
