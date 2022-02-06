/*
Copyright 2017 The Kubernetes Authors.
Copyright 2022 Johannes 'fish' Ziemke.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Image is a specification for a Image resource
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Status")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:subresource:status
type Image struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ImageSpec `json:"spec"`
	// +optional
	Status ImageStatus `json:"status",omitempty`
}

// ImageSpec is the spec for a Image resource
type ImageSpec struct {
	BuilderName   string `json:"builderName"`
	Containerfile string `json:"containerfile"`
	Registry      string `json:"registry"`
	Repository    string `json:"repository"`
	Tag           string `json:"tag"`
	// +optional
	BuildArgs []BuildArg `json:"buildArgs"`
}

type BuildArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ImageCondition struct {
	Type               ImageConditionType `json:"type"`
	Status             v1.ConditionStatus `json:"status"`
	LastTransitionTime *metav1.Time       `json:"lastTransitionTime,omitempty"`
	Reason             string             `json:"reason,omitempty"`
	Message            string             `json:"message,omitempty"`
}

type ImageConditionType string

// ImageStatus is the status for a Image resource
type ImageStatus struct {
	Image      string           `json:"image"`
	Conditions []ImageCondition `json:"conditions,omitempty"`
}

// ImageList is a list of Image resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Image `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:generateEmbeddedObjectMeta=true
// +genclient
type ImageBuilder struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Template          corev1.PodTemplateSpec `json:"template",omitempty`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
type ImageBuilderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ImageBuilder `json:"items"`
}
