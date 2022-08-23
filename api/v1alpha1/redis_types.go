// Copyright 2019 The redis-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Redis is the Schema for the redis API
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="Master",type="string",JSONPath=".status.master",description="Current master's Pod name"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas",description="Current number of Redis instances"
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".spec.replicas",description="Desired number of Redis instances"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
type Redis struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSpec   `json:"spec"`
	Status RedisStatus `json:"status,omitempty"`
}

// RedisSpec defines the desired state of Redis
type RedisSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

	// Replicas is a number of replicas in a Redis failover cluster
	// +kubebuilder:validation:Minimum=3
	Replicas *int32 `json:"replicas"`

	// Config allows to pass custom Redis configuration parameters
	Config   map[string]string `json:"config,omitempty"`
	Password Password          `json:"password,omitempty"`

	// Pod annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Pod securityContext
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// Pod affinity
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// NodeSelector specifies a map of key-value pairs. For the pod to be
	// eligible to run on a node, the node must have each of the indicated
	// key-value pairs as labels.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Pod tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Pod ServiceAccountName is the name of the ServiceAccount to use to run this pod.
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Pod ImagePullSecrets
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Pod priorityClassName
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// DataVolumeClaimTemplate for StatefulSet
	DataVolumeClaimTemplate corev1.PersistentVolumeClaim `json:"dataVolumeClaimTemplate,omitempty"`
	// Volumes for StatefulSet
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Redis container specification
	Redis ContainerSpec `json:"redis"`

	// Exporter container specification
	Exporter ContainerSpec `json:"exporter,omitempty"`

	// Pod initContainers
	InitContainers []corev1.Container `json:"initContainers,omitempty"`
}

// Password allows to refer to a Secret containing password for Redis
// Password should be strong enough. Passwords shorter than 8 characters
// composed of ASCII alphanumeric symbols will lead to a mild warning logged by the Operator.
// Please note that password hashes are added as annotations to Pods to enable
// password rotation. Hashes are generated using argon2id KDF.
// Changing the password in the referenced Secret will not trigger
// the rolling Statefulset upgrade automatically.
// However an event in regard to any objects owned by the Redis resource
// fired afterwards will trigger the rolling upgrade.
// Redis operator does not store the password internally and reads it
// from the Secret any time the Reconcile is called.
// Hence it will not be able to connect to Pods with the “old” password.
// In scenarios when persistence is turned off all the data will be lost
// during password rotation.
type Password struct {
	// SecretKeyRef is a reference to the Secret in the same namespace containing the password.
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef"`
}

// ContainerSpec allows to set some container-specific attributes
type ContainerSpec struct {
	// Image is a standard path for a Container image
	Image string `json:"image"`
	// Resources describes the compute resource requirements
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// SecurityContext holds security configuration that will be applied to a container
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
	// Number of seconds after the container has started before liveness probes are initiated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
}

// RedisStatus contains the observed state of Redis
type RedisStatus struct {
	// Replicas is the number of active Redis instances in the replication
	Replicas int `json:"replicas"`
	// Master is the current master's Pod name
	Master string `json:"master"`
}

// RedisList is a list of Redis resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	// +k8s:openapi-gen=false
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Redis resources
	Items []Redis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Redis{}, &RedisList{})
}
