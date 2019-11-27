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

package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var waterlog = logf.Log.WithName("water-resource")

func (r *Water) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-nuwa-nip-io-v1-water,mutating=true,failurePolicy=fail,groups=nuwa.nip.io,resources=waters,verbs=create;update,versions=v1,name=mwater.kb.io
var _ webhook.Defaulter = &Water{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Water) Default() {
	waterlog.Info("default", "name", r.Name)
	waterlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	//labels := map[string]string{"app": r.Name}
	//r.Spec.Deploy.Template.Labels = labels

	waterlog.Info("webhook working", "name", r.Name)
	//var cns []corev1.Container
	//cns = r.Spec.Deploy.Template.Spec.Containers
	//
	//container := corev1.Container{
	//	Name:  "water-sidecar-inject-nginx",
	//	Image: "nginx:1.12.2",
	//}
	//
	//cns = append(cns, container)
	//r.Spec.Deploy.Template.Spec.Containers = cns

	waterlog.Info("water nginx inject.")
	// TODO(user): fill in your defaulting logic.
	waterlog.Info("water webhook default", "name", r.Name)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-nuwa-nip-io-v1-water,mutating=false,failurePolicy=fail,groups=nuwa.nip.io,resources=waters,versions=v1,name=vwater.kb.io

var _ webhook.Validator = &Water{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Water) ValidateCreate() error {
	waterlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Water) ValidateUpdate(old runtime.Object) error {
	waterlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Water) ValidateDelete() error {
	waterlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
