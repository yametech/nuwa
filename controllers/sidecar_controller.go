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

	"k8s.io/apimachinery/pkg/labels"

	"github.com/go-logr/logr"
	"github.com/golang/glog"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	annotationPrefix = "sidecar.admission.nuwav1.io"
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
	//ctx := context.Background()
	//logf := r.Log.WithValues("sidecar", req.NamespacedName)

	// Fetch the PodPreset instance
	pp := &nuwav1.Sidecar{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, pp)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	glog.V(6).Infof("Fetched podpreset object: %#v\n", pp)

	selector, err := metav1.LabelSelectorAsSelector(&pp.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}
	podList := &corev1.PodList{}
	err = r.Client.List(context.TODO(), podList, &client.ListOptions{Namespace: pp.GetNamespace()})
	if err != nil {
		return reconcile.Result{}, err
	}
	for i, pod := range podList.Items {
		glog.V(6).Infof("(%v) Looking at pod %v\n", i, pod.Name)
		if selector.Matches(labels.Set(pod.Labels)) {
			n := pp.GetName()
			bouncedKey := fmt.Sprintf("%s/bounced-%s", annotationPrefix, n)
			_, found := pod.ObjectMeta.Annotations[bouncedKey]
			if !found {
				metav1.SetMetaDataAnnotation(&pod.ObjectMeta, bouncedKey, pp.GetResourceVersion())
				err = r.Client.Update(context.TODO(), &pod)
				if err != nil {
					return reconcile.Result{}, err
				}
			}

		}
	}
	return reconcile.Result{}, nil
	//deploymentList := &appsv1.DeploymentList{}
	//err = r.Client.List(context.TODO(), deploymentList, &client.ListOptions{})
	//if err != nil {
	//	return reconcile.Result{}, err
	//}
	//
	//for i, deployment := range deploymentList.Items {
	//	glog.V(6).Infof("(%v) Looking at deployment %v\n", i, deployment.Name)
	//	if selector.Matches(labels.Set(deployment.Spec.Template.ObjectMeta.Labels)) {
	//		bouncedKey := fmt.Sprintf("%s/bounced-%s", annotationPrefix, pp.GetName())
	//		resourceVersion, found := deployment.Spec.Template.ObjectMeta.Annotations[bouncedKey]
	//		if !found || found && resourceVersion < pp.GetResourceVersion() {
	//			// bounce pod since this is the first mutation or a later mutation has occurred
	//			glog.V(4).Infof("Detected deployment '%v' needs bouncing", deployment.Name)
	//			// TODO: may not need both of these events
	//			//r.recorder.Eventf(pp, v1.EventTypeNormal, "DeploymentBounced", "Bounced %v-%v due to newly created or updated podpreset", deployment.Name, deployment.GetResourceVersion())
	//			//r.recorder.Eventf(&deployment, v1.EventTypeNormal, "DeploymentBounced", "Bounced to newly created or updated podpreset %v-%v", pp.Name, pp.GetResourceVersion())
	//			metav1.SetMetaDataAnnotation(&deployment.Spec.Template.ObjectMeta, bouncedKey, pp.GetResourceVersion())
	//			err = r.Client.Update(context.TODO(), &deployment)
	//			if err != nil {
	//				return reconcile.Result{}, err
	//			}
	//		}
	//	}
	//}

	//instance := &nuwav1.Sidecar{}
	//if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
	//	if errors.IsNotFound(err) {
	//		return ctrl.Result{}, nil
	//	}
	//	return ctrl.Result{}, err
	//}
	//
	//if instance.DeletionTimestamp != nil {
	//	logf.Info("Resource is being deleted")
	//	return ctrl.Result{}, nil
	//}
	//
	//switch instance.Spec.ResourceType {
	//case nuwav1.WaterCRD:
	//	water := &nuwav1.Water{}
	//	if err := r.Client.Get(ctx,
	//		types.NamespacedName{Namespace: instance.Spec.NameSpace, Name: instance.Spec.Name},
	//		water); err != nil {
	//		if errors.IsNotFound(err) {
	//			return ctrl.Result{}, nil
	//		}
	//	}
	//	// 如果有water / .... 关联属主
	//
	//	podList := &corev1.PodList{}
	//	if err := r.Client.List(ctx, podList, client.MatchingLabels(water.Labels)); err != nil {
	//		if errors.IsNotFound(err) {
	//			return ctrl.Result{}, nil
	//		}
	//	}
	//
	//	for index := range podList.Items {
	//		pod := podList.Items[index]
	//		pod.Spec.Containers = append(pod.Spec.Containers, instance.Spec.Containers...)
	//	}
	//
	//	if err := r.Client.Update(ctx, water); err != nil {
	//		return ctrl.Result{}, err
	//	}
	//
	//case nuwav1.StoneCRD:
	//}
	//
	//return ctrl.Result{}, nil
}

func (r *SidecarReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nuwav1.Sidecar{}).
		Complete(r)
}
