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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nuwav1 "github.com/yametech/nuwa/api/v1"
)

// InjectorReconciler reconciles a Injector object
type InjectorReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=injectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=injectors/status,verbs=get;update;patch
func (r *InjectorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logf := r.Log.WithValues("injector", req.NamespacedName)
	// Fetch the PodPreset instance
	instance := &nuwav1.Injector{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			logf.Info("Nuwa sidecar resource not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if instance.Spec.Name == "" {
		return reconcile.Result{}, nil
	}
	if instance.DeletionTimestamp != nil {
		logf.Info("Resource is being deleted")
		return ctrl.Result{}, nil
	}
	logf.Info("Fetched nuwav1  injector object")

	if instance.Spec.ResourceType == "Water" {
		water := &nuwav1.Water{}
		if err := r.Client.Get(ctx,
			types.NamespacedName{Namespace: instance.Spec.NameSpace, Name: instance.Spec.Name},
			water); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
		}
		instance.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(
				water,
				schema.GroupVersionKind{
					Group:   nuwav1.GroupVersion.Group,
					Version: nuwav1.GroupVersion.Version,
					Kind:    "Water",
				}),
		}

	}

	if instance.Spec.ResourceType == "Stone" {
		water := &nuwav1.Stone{}
		if err := r.Client.Get(ctx,
			types.NamespacedName{Namespace: instance.Spec.NameSpace, Name: instance.Spec.Name},
			water); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
		}
		instance.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(
				water,
				schema.GroupVersionKind{
					Group:   nuwav1.GroupVersion.Group,
					Version: nuwav1.GroupVersion.Version,
					Kind:    "Stone",
				}),
		}
	}

	selector, err := metav1.LabelSelectorAsSelector(&instance.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}
	podList := &corev1.PodList{}
	err = r.Client.List(context.TODO(), podList, &client.ListOptions{Namespace: instance.GetNamespace()})
	if err != nil {
		return reconcile.Result{}, err
	}
	for _, pod := range podList.Items {
		if selector.Matches(labels.Set(pod.Labels)) {
			n := instance.GetName()
			bouncedKey := fmt.Sprintf("%s/bounced-%s", annotationPrefix, n)
			_, found := pod.ObjectMeta.Annotations[bouncedKey]
			if !found {
				metav1.SetMetaDataAnnotation(&pod.ObjectMeta, bouncedKey, instance.GetResourceVersion())
				err = r.Client.Update(context.TODO(), &pod)
				if err != nil {
					return reconcile.Result{}, err
				}
			}

		}
	}
	return reconcile.Result{}, nil
}

func (r *InjectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Injector{}).
		Complete(r)
}
