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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// InjectorSpec defines the desired state of Injector
type InjectorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	PreContainers []corev1.Container `json:"preContainers,omitempty"`

	AfterContainers []corev1.Container `json:"afterContainers,omitempty"`

	ResourceType string `json:"resourceType,omitempty"`

	NameSpace string `json:"namespace,omitempty"`

	Name string `json:"name,omitempty"`
	// Selector is a label query over a set of resources, in this case pods.
	// Required.
	Selector metav1.LabelSelector `json:"selector"`
}

// InjectorStatus defines the observed state of Injector
type InjectorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// Injector is the Schema for the injectors API
type Injector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InjectorSpec   `json:"spec,omitempty"`
	Status InjectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InjectorList contains a list of Injector
type InjectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Injector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Injector{}, &InjectorList{})
}
