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
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
)

// WaterReconciler reconciles a Water object
type WaterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func deploymentName(coordinateName string, instance *nuwav1.Water) string {
	return fmt.Sprintf("%s-%s", instance.Name, strings.ToLower(coordinateName))
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters/status,verbs=get;update;patch
func (r *WaterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	objectKey := req.NamespacedName
	logf := r.Log.WithValues("water", objectKey.String(), "namespace", req.Namespace)
	ctx := context.WithValue(context.TODO(), "request", req)

	instance := &nuwav1.Water{}
	if err := r.Client.Get(ctx, objectKey, instance); err != nil {
		if errors.IsNotFound(err) {
			logf.Info("not found water")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	//if instance.DeletionTimestamp != nil {
	//	logf.Info("Resource is being deleted")
	//	return ctrl.Result{}, nil
	//}

	//  TODO fill logic
	coordinators, err := makeLocalCoordinates(r.Client, instance.Spec.Coordinates)
	if err != nil {
		return ctrl.Result{}, err
	}

	for i := range coordinators {
		lc := coordinators[i]
		logf.Info("deployment for", "coordinate", lc.Name)

		objKey := types.NamespacedName{Namespace: req.Namespace, Name: deploymentName(lc.Name, instance)}
		deployment := &appsv1.Deployment{}
		if err := r.Client.Get(ctx, objKey, deployment); err != nil {
			if errors.IsNotFound(err) {
				if err := r.createDeployment(ctx, objKey, instance, &lc.Coordinate.Replicas, lc.NodeAffinity); err != nil {
					return ctrl.Result{}, err
				}
				if &instance.Status == nil {
					instance.Status = nuwav1.WaterStatus{}
				}
				instance.Status.Current += 1
				if err := r.createService(ctx, instance); err != nil {
					return ctrl.Result{}, err
				}
				// Annotations
				r.annotation(instance)
				if err := r.Client.Update(context.TODO(), instance); err != nil {
					return reconcile.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *WaterReconciler) annotation(instance *nuwav1.Water) {
	data, _ := json.Marshal(instance.Spec)
	if instance.Annotations != nil {
		instance.Annotations["spec"] = string(data)
	} else {
		instance.Annotations = map[string]string{"spec": string(data)}
	}
}

func (r *WaterReconciler) createService(ctx context.Context, instance *nuwav1.Water) error {
	objectKey := ctx.Value("request").(ctrl.Request).NamespacedName
	nameSpace := ctx.Value("request").(ctrl.Request).Namespace
	logf := r.Log.WithValues("water create service", objectKey, "namespace", nameSpace)

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, schema.GroupVersionKind{
					Group:   nuwav1.GroupVersion.Group,
					Version: nuwav1.GroupVersion.Version,
					Kind:    "Water"}),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     instance.Spec.Service.Type,
			Ports:    instance.Spec.Service.Ports,
			Selector: map[string]string{"app": instance.Name},
		},
	}

	if err := r.Client.Get(ctx, objectKey, service); err != nil {
		if errors.IsNotFound(err) {
			if err = r.Client.Create(ctx, service); err != nil {
				return err
			}
		} else if errors.IsAlreadyExists(err) {
			logf.Info("Service", "status", "already exists")
		} else {
			return err
		}
	}

	logf.Info("Create service", "status", "done!!!")
	return nil
}

func (r *WaterReconciler) createDeployment(ctx context.Context, deployName types.NamespacedName, instance *nuwav1.Water, size *int32, nodeAffinity *corev1.NodeAffinity) error {
	objectKey := ctx.Value("request").(ctrl.Request).NamespacedName
	nameSpace := ctx.Value("request").(ctrl.Request).Namespace
	logf := r.Log.WithValues("water create deployment", objectKey, "namespace", nameSpace)

	labels := map[string]string{"app": instance.Name}
	selector := &metav1.LabelSelector{MatchLabels: labels}
	if nodeAffinity != nil {
		instance.Spec.Deploy.Template.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: nodeAffinity,
		}
	}

	newDeploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName.Name,
			Namespace: deployName.Namespace,
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
			Replicas: size,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       instance.Spec.Deploy.Template.Spec,
			},
			Selector: selector,
		},
	}

	if err := r.Client.Create(ctx, newDeploy); err != nil {
		return err
	}
	// Set Deployment instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, newDeploy, r.Scheme); err != nil {
		return err
	}

	oldDeploy := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, deployName, oldDeploy); err != nil {
		if !errors.IsNotFound(err) {
			oldDeploy.Spec = newDeploy.Spec
			if err := r.Client.Update(ctx, oldDeploy); err != nil {
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

func (r *WaterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Water{}).Owns(&appsv1.Deployment{}).Complete(r)
}
