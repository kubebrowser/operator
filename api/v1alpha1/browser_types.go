/*
Copyright 2025.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BrowserSpec defines the desired state of Browser
type BrowserSpec struct {

	// Whether or not the deployment should have running instances.
	// +kubebuilder:default:=true
	// +optional
	Started *bool `json:"started,omitempty" protobuf:"bytes,8,opt,name=started"`

	// Resources requirements for the browser container.
	// +optional
	BrowserResources corev1.ResourceRequirements `json:"browserResources,omitempty" protobuf:"bytes,8,opt,name=browserResources"`

	// +optional
	// future feature
	// PersistentStorageSize resource.Quantity `json:"persistentStorageSize,omitempty" protobuf:"bytes,8,opt,name=persistentStorageSize"`

	// +optional
	// future feature
	// window-resolution ? widthxheight?
}

// BrowserStatus defines the observed state of Browser
type BrowserStatus struct {
	// For further information see: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Most recently observed status of the Deployment.
	DeploymentStatus string `json:"deploymentStatus,omitempty" protobuf:"bytes,1,opt,name=deploymentStatus"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:action
// +kubebuilder:subresource:vnc
// +kubebuilder:printcolumn:name="Started",type=boolean,JSONPath=`.spec.started`
// +kubebuilder:printcolumn:name="Deployment",type=string,JSONPath=`.status.deploymentStatus`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Browser is the Schema for the browsers API
type Browser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BrowserSpec   `json:"spec,omitempty"`
	Status BrowserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BrowserList contains a list of Browser
type BrowserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Browser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Browser{}, &BrowserList{})
}
