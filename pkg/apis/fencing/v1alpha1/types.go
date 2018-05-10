package v1alpha1

// https://github.com/kubernetes/apimachinery/blob/master/pkg/apis/meta/v1/types.go

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
}

type FencingRequestStatus struct {
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
	Spec              FencingSetSpec   `json:"spec"`
	Status            FencingSetStatus `json:"status,omitempty"`
}


type FencingSetSpec struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	Methods    []FencingMechanism `json:"methods"`
	MethodRefs []v1.LocalObjectReference `json:"methodRefs"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingMethodList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []FencingMethod `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingMethod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FencingMethodSpec   `json:"spec"`
	Status            FencingMethodStatus `json:"status,omitempty"`
}

type FencingMethodSpec struct {
	Mechansims []FencingMechanism `json:"mechanisms"`
}

type FencingMechanism struct {
	Driver         string `json:"driver"`
	Module         string `json:"module"`
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
  
