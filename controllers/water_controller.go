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
	"fmt"
	"github.com/go-logr/logr"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CoordinateErr error

var (
	NoDefinedErr CoordinateErr = fmt.Errorf("not defined")
)

// WaterReconciler reconciles a Water object
type WaterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters/status,verbs=get;update;patch
func (r *WaterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	objectKey := req.NamespacedName
	logf := r.Log.WithValues("water", objectKey.String(), "namespace", req.Namespace)
	ctx := context.WithValue(context.TODO(), "request", req)

	instance := &nuwav1.Water{}
	if err := r.Client.Get(ctx, objectKey, instance); err != nil {
		if errors.IsNotFound(err) {
			logf.Info("Resource not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		logf.Info("Resource is being deleted")
		return ctrl.Result{}, nil
	}

	deployment := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, objectKey, deployment); err != nil {
		if errors.IsNotFound(err) {
			if err = r.createDeployment(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}

			if err = r.createService(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
			// Annotations
			data, _ := json.Marshal(instance.Spec)
			if instance.Annotations != nil {
				instance.Annotations["spec"] = string(data)
			} else {
				instance.Annotations = map[string]string{"spec": string(data)}
			}

			if err = r.Client.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *WaterReconciler) createService(ctx context.Context, instance *nuwav1.Water) error {
	logf := r.Log.WithValues("water create service", ctx.Value("request").(ctrl.Request).NamespacedName.String(), "namespace", ctx.Value("request").(ctrl.Request).Namespace)

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(
					instance,
					schema.GroupVersionKind{
						Group:   nuwav1.GroupVersion.Group,
						Version: nuwav1.GroupVersion.Version,
						Kind:    "Water",
					}),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			// TODO
			//Ports: instance.Spec.Deploy.Template.,
			Selector: map[string]string{
				"app": instance.Name,
			},
		},
	}

	if err := r.Client.Get(ctx, ctx.Value("request").(ctrl.Request).NamespacedName, service); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if err = r.Client.Create(ctx, service); err != nil {
			return err
		}
	}

	logf.Info("Create service", "status", "done!!!")
	return nil
}

func (r *WaterReconciler) createDeployment(ctx context.Context, instance *nuwav1.Water) error {
	logf := r.Log.WithValues("water create deployment",
		ctx.Value("request").(ctrl.Request).NamespacedName.String(),
		"namespace", ctx.Value("request").(ctrl.Request).Namespace,
	)

	labels := map[string]string{"app": instance.Name}
	selector := &metav1.LabelSelector{MatchLabels: labels}
	size := int32(1)

	newDeploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance,
					schema.GroupVersionKind{
						Group:   nuwav1.GroupVersion.Group,
						Version: nuwav1.GroupVersion.Version,
						Kind:    "Water",
					}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &size,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: instance.Spec.Deploy.Template.Spec,
			},
			Selector: selector,
		},
	}

	if err := r.Client.Create(ctx, newDeploy); err != nil {
		logf.Error(err, "Deployment create error")
		return err
	}

	// Set Deployment instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, newDeploy, r.Scheme); err != nil {
		logf.Error(err, "SetControllerReference error")
		return err
	}

	oldDeploy := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, ctx.Value("request").(ctrl.Request).NamespacedName, oldDeploy); err != nil {
		if errors.IsNotFound(err) {
			logf.Info("Get old deployment not found")
		} else {
			oldDeploy.Spec = newDeploy.Spec
			if err := r.Client.Update(ctx, oldDeploy); err != nil {
				logf.Error(err, "Update lod deployment error")
				return err
			}
		}
	}

	logf.Info("Create deployment", "status", "done!!!")
	return nil
}

func (r *WaterReconciler) updateWater(ctx context.Context, instance *nuwav1.Water) error {
	old := &nuwav1.Water{}
	if err := json.Unmarshal([]byte(instance.Annotations["spec"]), old); err != nil {
		return err
	}
	if !reflect.DeepEqual(instance.Spec, old.Spec) {
		// Update associated resources
		old.Spec = instance.Spec
		if err := r.Client.Update(ctx, old); err != nil {
			return err
		}
	}
	return nil
}

func (r *WaterReconciler) findMatchNodes(ctx context.Context, instance *nuwav1.Water) (map[string]corev1.Node, error) {
	if len(instance.Spec.Coordinates) < 1 {
		return nil, NoDefinedErr
	}
	for index := range instance.Spec.Coordinates {
		coordinate := instance.Spec.Coordinates[index]

		labels := map[string]string{
			"nuwa.io/room":    coordinate.Room,
			"nuwa.io/cabinet": coordinate.Cabinet,
		}
		selector := &metav1.LabelSelector{MatchLabels: labels}
		_ = selector
		// TODO//

		nodeList := &corev1.NodeList{}
		if err := r.Client.List(ctx, nodeList); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (r *WaterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Water{}).
		Complete(r)
}
