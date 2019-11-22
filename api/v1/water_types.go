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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type StrategyType string

const (
	Alpha   StrategyType = "Alpha"
	Beta    StrategyType = "Beta"
	Release StrategyType = "Release"
)

// WaterSpec defines the desired state of Water
type WaterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Copies.
	// +optional
	Copies *int32 `json:"copies,omitempty"`
	// Strategy
	// @Alpha : Only one node that meets expectations is selected to publish 1 application
	// @Beta  : All node that meets expectations is selected to publish each node 1 application
	// @Release : The number of copies(Replicas) based on beta release more than nodes will be published evenly in the nodes that conform to the specification
	// +optional
	// +patchStrategy=retainKeys
	// +patchMergeKey=type
	Strategy StrategyType `json:"strategy,omitempty"`
	// Identify the deployment status expected by the current resource
	// Identify node params ROOM-{N}_CABINET-{N}_HOST-{N}
	// +optional
	// +patchStrategy=retainKeys
	// +patchMergeKey=type
	Coordinates []Coordinate `json:"coordinates,omitempty"`
	// Identify the deployment expose expected by the current resource
	// +optional
	// +patchStrategy=retainKeys
	// +patchMergeKey=type
	AutoExpose bool `json:"autoExpose,omitempty"`
	//// Identify the deployment service expected by the current resource
	//// +optional
	Service corev1.ServiceSpec `json:"service,omitempty"`
	// +optional
	Deploy appsv1.DeploymentSpec `json:"deploy,omitempty"`
}

// WaterStatus defines the observed state of Water
type WaterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +k8s:openapi-gen=true
// Water is the Schema for the waters API
type Water struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WaterSpec   `json:"spec,omitempty"`
	Status WaterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WaterList contains a list of Water
type WaterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Water `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Water{}, &WaterList{})
}
