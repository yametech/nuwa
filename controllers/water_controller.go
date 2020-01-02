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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// WaterReconciler reconciles a Water object
type WaterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nuwa.nip.io,resources=waters/status,verbs=get;update;patch
func (r *WaterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	objectKey := req.NamespacedName
	logf := r.Log.WithValues("water_injector", objectKey.String(), "namespace", req.Namespace)
	ctx := context.WithValue(context.TODO(), "request", req)

	instance := &nuwav1.Water{}
	if err := r.Client.Get(ctx, objectKey, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	//  TODO fill logic

	if err := r.updateCleanOldDeployment(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	coordinators, err := makeLocalCoordinates(r.Client, instance.Spec.Coordinates)
	if err != nil {
		return ctrl.Result{}, err
	}

	for i := range coordinators {
		local := coordinators[i]
		logf.Info("Reconcile coordinate", "coordinate", local.Name)
		size := int32(1)
		switch instance.Spec.Strategy {
		case nuwav1.Alpha:
			if local.Index > 0 {
				continue
			}
			logf.Info("Water deploy strategy is Alpha")
		case nuwav1.Beta:
			logf.Info("Water deploy strategy is Beta")
		case nuwav1.Release:
			size = local.Coordinate.Replicas
			logf.Info("Water deploy strategy is Release")
		}
		objKey := client.ObjectKey{
			Namespace: req.Namespace,
			Name:      deploymentName(local.Name, instance),
		}
		if err := r.updateDeployment(ctx, objKey, instance, &size, local.NodeAffinity); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.createService(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.annotationWater(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.updateWater(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *WaterReconciler) annotationWater(ctx context.Context, instance *nuwav1.Water) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		data, err := json.Marshal(instance.Spec)
		if err != nil {
			return err
		}
		if instance.Annotations != nil {
			instance.Annotations["spec"] = string(data)
		} else {
			instance.Annotations = map[string]string{"spec": string(data)}
		}
		if err := r.Client.Update(ctx, instance); err != nil {
			return err
		}
		return nil
	})
}

func (r *WaterReconciler) createService(ctx context.Context, instance *nuwav1.Water) error {
	objectKey := ctx.Value("request").(ctrl.Request).NamespacedName
	nameSpace := ctx.Value("request").(ctrl.Request).Namespace
	logf := r.Log.WithValues("water_injector create service", objectKey, "namespace", nameSpace)

	serviceSpec := instance.Spec.Service.DeepCopy()
	serviceSpec.Selector = map[string]string{"app": instance.Name}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            instance.Name,
			Namespace:       instance.Namespace,
			OwnerReferences: ownerReference(instance, "Water"),
		},
		Spec: *serviceSpec,
	}

	logf.Info("Create service", "service", instance.Name)
	if err := r.Client.Get(ctx, objectKey, service); err != nil {
		if errors.IsNotFound(err) {
			if err = r.Client.Create(ctx, service); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	return nil
}

func (r *WaterReconciler) updateDeployment(ctx context.Context, deployName client.ObjectKey, instance *nuwav1.Water, size *int32, nodeAffinity *corev1.NodeAffinity) error {
	labels := map[string]string{"app": instance.Name}
	selector := &metav1.LabelSelector{MatchLabels: labels}
	newTemplate := instance.Spec.Template.DeepCopy()
	if nodeAffinity != nil {
		newTemplate.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: nodeAffinity,
		}
	}

	newDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            deployName.Name,
			Namespace:       deployName.Namespace,
			OwnerReferences: ownerReference(instance, "Water"),
		},

		Spec: appsv1.DeploymentSpec{
			Replicas: size,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       newTemplate.Spec,
			},
			Selector: selector,
		},
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		oldDeployment := &appsv1.Deployment{}
		if err := r.Client.Get(ctx, deployName, oldDeployment); err != nil {
			if errors.IsNotFound(err) {
				if err := r.Client.Create(ctx, newDeployment); err != nil {
					return err
				}
				// Set Deployment instance as the owner and controller
				if err := controllerutil.SetControllerReference(instance, newDeployment, r.Scheme); err != nil {
					return err
				}
				return nil
			}
			return err
		}

		if *oldDeployment.Spec.Replicas != *size {
			*oldDeployment.Spec.Replicas = *size
			if err := r.Client.Update(ctx, oldDeployment); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *WaterReconciler) updateWater(ctx context.Context, instance *nuwav1.Water) error {
	old := &nuwav1.Water{}
	objKey := client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}
	if err := r.Client.Get(ctx, objKey, old); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		expectStatus := nuwav1.WaterStatus{}
		coordinators, err := makeLocalCoordinates(r.Client, instance.Spec.Coordinates)
		if err != nil {
			return err
		}
		expectStatus.DesiredDeployments = int32(len(coordinators))
		for i := range coordinators {
			local := coordinators[i]
			expectStatus.DesiredReplicas += local.Coordinate.Replicas
			objKey := client.ObjectKey{Namespace: instance.Namespace, Name: deploymentName(local.Name, instance)}
			tmp := &appsv1.Deployment{}
			if err := r.Client.Get(ctx, objKey, tmp); err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}
			expectStatus.AlreadyReplicas += *tmp.Spec.Replicas
			expectStatus.AlreadyDeployment++
		}
		if !reflect.DeepEqual(old.Status, expectStatus) {
			old.Status = expectStatus
			if err := r.Client.Status().Update(ctx, old); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *WaterReconciler) updateCleanOldDeployment(ctx context.Context, instance *nuwav1.Water) error {
	if instance.Annotations == nil {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var tmpWaterSpec nuwav1.WaterSpec
		bs, ok := instance.Annotations["spec"]
		if !ok {
			return nil
		}
		if err := json.Unmarshal([]byte(bs), &tmpWaterSpec); err != nil {
			return err
		}

		tmp1 := tmpWaterSpec.Coordinates.DeepCopy()
		tmp2 := instance.Spec.Coordinates.DeepCopy()
		diffSlice := nuwav1.Difference(tmp1, tmp2)
		if len(diffSlice) > 0 {
			for _, c := range tmp1 {
				if nuwav1.In(tmp1, c) && !nuwav1.In(tmp2, c) {
					r.Log.Info("Delete", "room", c.Room, "cabinet", c.Cabinet, "host", c.Host)
					coorName, err := coordinateName(&c)
					if err != nil {
						return err
					}
					objKey := client.ObjectKey{
						Namespace: instance.Namespace,
						Name:      deploymentName(coorName, instance),
					}
					deployment := &appsv1.Deployment{}
					if err := r.Client.Get(ctx, objKey, deployment); err != nil {
						if errors.IsNotFound(err) {
							continue
						}
						return err
					}
					if err := r.Client.Delete(ctx, deployment); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func (r *WaterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Water{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
