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
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	//_ "sort"
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
			logf.Info("receive request could not be found stone resource or non specified resources",
				"namespace",
				req.Namespace, "name", req.Name,
			)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.syncStatefulSet(ctx, logf, ste); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateStone(ctx, logf, ste); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *StoneReconciler) getStatefulSet(ctx context.Context, log logr.Logger, cgs []nuwav1.CoordinatesGroup, ste *nuwav1.Stone) ([]*nuwav1.StatefulSet, error) {
	stsPointerSlice := make([]*nuwav1.StatefulSet, 0)

	for i := range cgs {
		statefulSetName := statefulSetName(ste, cgs[i].Group, i)
		sts := &nuwav1.StatefulSet{}
		key := client.ObjectKey{Name: statefulSetName, Namespace: ste.Namespace}

		// default is the number of machines per group
		var size int32
		maxUnavailable := intstr.FromInt(1)
		partition := int32(0) // default 0

		switch ste.Spec.Strategy {
		case nuwav1.Alpha:
			size = 1
			if i > 0 {
				continue
			}
		case nuwav1.Beta:
			size = 1
		case nuwav1.Omega:
			size = int32(cgs[i].Zoneset.Len())
		case nuwav1.Release:
			size = *cgs[i].Replicas
		default:
			ste.Spec.Strategy = nuwav1.Alpha
			size = 1
			log.Info("stone strategy definition is incorrect or exceeded")
		}

		if err := r.Client.Get(ctx, key, sts); err != nil {
			if errors.IsNotFound(err) {
				// structure new statefulset
				labels := map[string]string{
					"app": ste.GetName(),
				}
				for k, v := range ste.Labels {
					labels[k] = v
				}
				selector := &metav1.LabelSelector{MatchLabels: labels}
				fakeServiceName := "fake"
				zoneSetAnnotations, _ := json.Marshal(cgs[i].Zoneset)
				sts = &nuwav1.StatefulSet{
					TypeMeta: metav1.TypeMeta{APIVersion: "nuwa.nip.io/v1", Kind: "StatefulSet"},
					ObjectMeta: metav1.ObjectMeta{
						Name:            statefulSetName,
						Namespace:       ste.Namespace,
						OwnerReferences: ownerReference(ste, "Stone"),
					},
					Spec: nuwav1.StatefulSetSpec{
						ServiceName: fakeServiceName + "-" + statefulSetName,
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: labels,
								Annotations: map[string]string{
									"coordinates": string(zoneSetAnnotations),
								},
							},
							Spec: ste.Spec.Template.Spec,
						},
						Selector:            selector,
						PodManagementPolicy: apps.ParallelPodManagement,
						UpdateStrategy: &nuwav1.StatefulSetUpdateStrategy{
							Type: apps.RollingUpdateStatefulSetStrategyType,
							RollingUpdate: &nuwav1.RollingUpdateStatefulSetStrategy{
								MaxUnavailable:  &maxUnavailable,
								Partition:       &partition,
								PodUpdatePolicy: nuwav1.InPlaceIfPossiblePodUpdateStrategyType,
							},
						},
						VolumeClaimTemplates: ste.Spec.VolumeClaimTemplates,
					},
				}

				sts.Spec.Template.Spec.ReadinessGates = make([]corev1.PodReadinessGate, 1)
				sts.Spec.Template.Spec.ReadinessGates[0] = corev1.PodReadinessGate{ConditionType: "InPlaceUpdateReady"}

				log.Info(
					"statefulSet not found",
					"namespace",
					ste.Namespace,
					"statefulSetName",
					statefulSetName,
				)
			} else {
				return nil, err
			}
		}

		sts.Spec.Replicas = &size
		sts.Spec.Template.Spec.Containers = ste.Spec.Template.Spec.Containers
		sts.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &maxUnavailable
		sts.Spec.UpdateStrategy.RollingUpdate.Partition = &partition

		stsPointerSlice = append(stsPointerSlice, sts)
	}

	return stsPointerSlice, nil
}

func checkService(service *corev1.Service) bool {
	if len(service.Spec.Ports) < 1 {
		return false
	}
	for _, port := range service.Spec.Ports {
		port.Name = strings.Replace(
			strings.Replace(
				strings.ToLower(port.Name),
				"_", "-", -1),
			".", "", -1)
	}
	return true
}

func (r *StoneReconciler) updateService(ctx context.Context, log logr.Logger, ste *nuwav1.Stone, sts *nuwav1.StatefulSet) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		tmp := &corev1.Service{}
		key := client.ObjectKey{Name: sts.Name, Namespace: sts.Namespace}
		err := r.Client.Get(ctx, key, tmp)
		if err != nil {
			if errors.IsNotFound(err) {
				// Create service
				// check port dns name
				serviceSpec := ste.Spec.Service.DeepCopy()
				serviceSpec.Selector = map[string]string{"app": ste.Name}

				service := &corev1.Service{
					TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
					ObjectMeta: metav1.ObjectMeta{
						Name:            sts.Name,
						Namespace:       sts.Namespace,
						OwnerReferences: ownerReference(ste, "Stone"),
					},
					Spec: *serviceSpec,
				}

				if !checkService(service) {
					return nil
				}

				if err = r.Client.Create(ctx, service); err != nil {
					return err
				}
				log.Info("Create statefulSet service", "service", sts.Name)
			} else {
				return err
			}
		}

		return nil
	})
}

func (r *StoneReconciler) updateStatefulSet(ctx context.Context, log logr.Logger, sts *nuwav1.StatefulSet, ste *nuwav1.Stone) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		tmp := &nuwav1.StatefulSet{}
		key := client.ObjectKey{Namespace: sts.Namespace, Name: sts.Name}

		err := r.Client.Get(ctx, key, tmp)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Info(
					"updateStatefulSet not found, create it",
					"namespace",
					sts.Namespace,
					"statefulset",
					sts.Name,
				)
				if err := r.Client.Create(ctx, sts); err != nil {
					log.Info(
						"updateStatefulSet create",
						"namespace",
						sts.Namespace,
						"statefulset",
						sts.Name,
						"error",
						err.Error(),
					)
					return err
				}

				if err := controllerutil.SetControllerReference(ste, sts, r.Scheme); err != nil {
					return err
				}

				return nil

			} else {
				log.Info(
					"updateStatefulSet unknow error",
					"namespace",
					sts.Namespace,
					"statefulset",
					sts.Name,
					"error",
					err.Error(),
				)

				return err
			}
		}

		if *tmp.Spec.Replicas != *sts.Spec.Replicas || !reflect.DeepEqual(tmp.Spec.Template.Spec, sts.Spec.Template.Spec) {
			if err := r.Client.Update(ctx, sts); err != nil {
				log.Info(
					"updateStatefulSet update error",
					"namespace",
					sts.Namespace,
					"statefulset",
					sts.Name,
					"error",
					err.Error(),
				)
				return err
			}
		}

		return nil
	})
}

func (r *StoneReconciler) syncStatefulSet(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		statefulSets, err := r.getStatefulSet(ctx, log, ste.Spec.Coordinates, ste)
		if err != nil {
			return err
		}
		for i := range statefulSets {
			sts := statefulSets[i]
			if err := r.updateStatefulSet(ctx, log, sts, ste); err != nil {
				return err
			}
		}
		for i := range statefulSets {
			sts := statefulSets[i]
			if err := r.updateService(ctx, log, ste, sts); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *StoneReconciler) updateStone(ctx context.Context, log logr.Logger, ste *nuwav1.Stone) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		bytes, err := json.Marshal(ste.Spec)
		if err != nil {
			return err
		}
		if ste.Annotations == nil {
			ste.Annotations = map[string]string{"spec": string(bytes)}
		}
		ste.Annotations["spec"] = string(bytes)
		if err = r.Client.Update(ctx, ste); err != nil {
			return err
		}

		statefulSets, err := r.getStatefulSet(ctx, log, ste.Spec.Coordinates, ste)
		if err != nil {
			return err
		}
		ste.Status.Replicas = 0
		ste.Status.StatefulSet = int32(len(statefulSets))
		for i := range statefulSets {
			ste.Status.Replicas += statefulSets[i].Status.Replicas
		}
		if err := r.Client.Status().Update(ctx, ste); err != nil {
			return err
		}

		return nil
	})
}

func (r *StoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Stone{}).
		Owns(&nuwav1.StatefulSet{}).
		Complete(r)
}
