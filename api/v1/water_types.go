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
	// Template describes the pods that will be created.
	Template corev1.PodTemplateSpec `json:"template"`
	// Strategy
	// @Alpha : Only one node that meets expectations is selected to publish 1 application
	// @Beta  : All node that meets expectations is selected to publish each node 1 application
	// @Release : The number of copies(Replicas) based on beta release more than nodes will be published evenly in the nodes that conform to the specification
	// +optional
	// +patchStrategy=retainKeys
	// +patchMergeKey=type
	Strategy StrategyType `json:"strategy"`
	// Identify the deployment status expected by the current resource
	// Identify node params ROOM-{N}_CABINET-{N}_HOST-{N}
	// +optional
	Coordinates []Coordinate `json:"coordinates,omitempty"`
	// Identify the deployment service expected by the current resource
	// +optional
	Service corev1.ServiceSpec `json:"service,omitempty"`
}

// WaterStatus defines the observed state of Water
type WaterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// desired number of pods
	Desired int32 `json:"desired"`
	// already replicas number of pods
	Replicas int32 `json:"replicas"`
}

/*
	kubebuilder:subresource:scale:specpath=.spec.copies,statuspath=.status.current,selectorpath=.spec.selector
*/

// +kubebuilder:object:root=true
// +k8s:openapi-gen=true
// Water is the Schema for the waters API
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wts
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".status.desired",description="The desired number of pods."
// +kubebuilder:printcolumn:name="REPLICAS",type="integer",JSONPath=".status.replicas",description="The already replicas number of pods."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
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
