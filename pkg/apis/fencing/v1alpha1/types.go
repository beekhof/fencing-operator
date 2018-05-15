package v1alpha1

// https://github.com/kubernetes/apimachinery/blob/master/pkg/apis/meta/v1/types.go

import (
	"date"
	
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NodeFenceConditionRunning means the node fencing is being executed
	RequestFailedNoConfig = "Running"
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []FencingRequest `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FencingRequestSpec   `json:"spec"`
	Status            FencingRequestStatus `json:"status,omitempty"`
}

type FencingRequestSpec struct {
	Target    string    `json:"target"`
	Origin    string    `json:"origin,omitempty"`	
	Operation string    `json:"operation,omitempty"`   // On, Off, Cycle

	// Allow actions to be scheduled in the future
	//
	// A _failed_ node returning to a healthy state cancels any
	// existing scheduled fencing actions
	ValidAfter date.Time `json:"validAfter,omitempty"`

}

type FencingRequestStatus struct {
	Complete  bool  `json:"complete"`
	Result    int  `json:"result"`
	Config *v1.LocalObjectReference `json:"config"`
	ActiveMethod *string `json:"activeMethod"`
	Updates []FencingRequestStatusUpdate `json:"updates,omitempty"`
}

type FencingRequestStatusUpdate struct {
	Timestamp date.Time `json:"timestamp"`
	Message   string `json:"message"`
	Output    string `json:"output"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []FencingSet `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FencingConfig   `json:"spec"`
	Status            FencingSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingConfig struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	Methods    []FencingMethod `json:"methods"`
}

type FencingMethod struct {
	Name string `json:"name"`
	RequireAfterSeconds *int32 `json:"requireAfterSeconds,omitempty"`
	Retries              int32 `json:"retries"`
	Mechansims []FencingMechanism `json:"mechanisms"`
}

type FencingMechanism struct {
	Driver    string `json:"driver"`
	Module    string `json:"module,omitempty"`

	PassTargetAs  string `json:"passTargetAs,omitempty"`

	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
	
	Config         map[string]string `json:"config"`
	DynamicConfig  []FencingDynamicConfig `json:"dynamicConfig"`

	// An optional list of references to secrets in the same namespace
	// to use when calling fencing modules (ie. IPMI or cloud provider passwords).
	// 
	// See https://kubernetes.io/docs/concepts/configuration/secret/ and/or
	// http://kubernetes.io/docs/user-guide/images#specifying-imagepullsecrets-on-a-pod
	Secrets map[string]v1.LocalObjectReference `json:"secrets"`
}

type FencingDynamicConfig struct {
	Field    string `json:"field"`
	Default  string `json:"default"`
	Values   map[string]string `json:"values"`
}

type FencingSetStatus struct {
	// Fill me
}

type FencingMethodStatus struct {
	// Fill me
}


//apiVersion: "fencing.clusterlabs.org/v1alpha1"
//kind: "FencingMethod"
//metadata:
//  name: "shared2"
//mechanisms: 
//  - driver: baremetal
//    module: ipmilan
//    dynamic_config:
//    - field: ip
//      values:
//      - somehost: 1.2.3.4
//      - otherhost: 1.2.3.5
//    config:
//    - user: admin
//    - passwordSecret: ipmi-secret
//---
//apiVersion: "fencing.clusterlabs.org/v1alpha1"
//kind: "FencingSet"
//metadata:
//  name: "shared-example"
//spec:
//  hosts:
//  - "*"
//  mechanismRefs: [ "shared1", "shared2" ]
  
