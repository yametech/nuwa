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

	"github.com/go-logr/logr"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

const annotationPrefix = "injector.admission.nuwav1.io"

// InjectorReconciler reconciles a Injector object
type InjectorReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=stones/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters/status,verbs=get;list;watch;create;update;patch;delete
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
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// TODO
	// webhook 中可能存在的问题,容器名冲突
	// 镜像拉取策略
	// water_injector or stone 创建时未触发reconcile --- 20191206194700 已解决

	if instance.Spec.Name == "" {
		return reconcile.Result{}, fmt.Errorf("%s", "required name is not defined")
	}

	logf.Info("Fetched nuwav1 injector object")

	objKey := types.NamespacedName{Namespace: instance.Spec.NameSpace, Name: instance.Spec.Name}
	switch instance.Spec.ResourceType {
	case "Water":
		water := &nuwav1.Water{}
		if err := r.Client.Get(ctx, objKey, water); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
			}
		}
		if instance.ObjectMeta.OwnerReferences == nil || len(instance.ObjectMeta.OwnerReferences) == 0 {
			instance.ObjectMeta.OwnerReferences = append(instance.ObjectMeta.OwnerReferences, ownerReference(water, "Water")...)
			if err := controllerutil.SetControllerReference(instance, water, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
		}

	case "Stone":
		stone := &nuwav1.Stone{}
		if err := r.Client.Get(ctx, objKey, stone); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
			}
		}
		if instance.ObjectMeta.OwnerReferences == nil || len(instance.ObjectMeta.OwnerReferences) == 0 {
			instance.ObjectMeta.OwnerReferences = append(instance.ObjectMeta.OwnerReferences, ownerReference(stone, "Stone")...)
			if err := controllerutil.SetControllerReference(instance, stone, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
		}

	default:

	}

	selector, err := metav1.LabelSelectorAsSelector(&instance.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}
	podList := &corev1.PodList{}
	err = r.Client.List(ctx, podList, &client.ListOptions{LabelSelector: selector, Namespace: instance.GetNamespace()})
	if err != nil {
		return reconcile.Result{}, err
	}
	for _, pod := range podList.Items {
		name := instance.GetName()
		bouncedKey := fmt.Sprintf("%s/bounced-%s", annotationPrefix, name)
		_, found := pod.ObjectMeta.Annotations[bouncedKey]
		if !found {
			metav1.SetMetaDataAnnotation(&pod.ObjectMeta, bouncedKey, instance.GetResourceVersion())
			err = r.Client.Update(context.TODO(), &pod)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	if err := r.Client.Update(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *InjectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Injector{}).
		Owns(&nuwav1.Water{}).
		Owns(&nuwav1.Stone{}).
		Complete(r)
}
