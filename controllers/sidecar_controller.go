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
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SidecarReconciler reconciles a Sidecar object
type SidecarReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=sidecars,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=sidecars/status,verbs=get;update;patch

func (r *SidecarReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logf := r.Log.WithValues("sidecar", req.NamespacedName)

	instance := &nuwav1.Sidecar{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		logf.Info("Resource is being deleted")
		return ctrl.Result{}, nil
	}

	switch instance.Spec.ResourceType {
	case nuwav1.WaterCRD:
		water := &nuwav1.Water{}
		if err := r.Client.Get(ctx,
			types.NamespacedName{Namespace: instance.Spec.NameSpace, Name: instance.Spec.Name},
			water); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
		}
		// 如果有water / .... 关联属主

		podList := &corev1.PodList{}
		if err := r.Client.List(ctx, podList, client.MatchingLabels(water.Labels)); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
		}

		for index := range podList.Items {
			pod := podList.Items[index]
			pod.Spec.Containers = append(pod.Spec.Containers, instance.Spec.Containers...)
		}

		if err := r.Client.Update(ctx, water); err != nil {
			return ctrl.Result{}, err
		}

	case nuwav1.StoneCRD:
	}

	return ctrl.Result{}, nil
}

func (r *SidecarReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Sidecar{}).
		Complete(r)
}
