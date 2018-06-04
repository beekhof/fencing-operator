package v1alpha1

// https://github.com/kubernetes/apimachinery/blob/master/pkg/apis/meta/v1/types.go

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FencingResult string

const (
	// NodeFenceConditionRunning means the node fencing is being executed
	RequestFailedNoConfig FencingResult = "NoConfig"
	RequestFailed         FencingResult = "GivingUp"
	MethodFailed          FencingResult = "MethodFailed"
	MethodComplete        FencingResult = "Complete"
	MethodProgress        FencingResult = "MethodInProgress"
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
	Target    string `json:"target"`
	Origin    string `json:"origin,omitempty"`
	Operation string `json:"operation,omitempty"` // On, Off, Cycle

	PodList []string `json:"podList,omitempty"` // A list of Pod's affected by this fencing event

	// Allow actions to be scheduled in the future
	//
	// A _failed_ node returning to a healthy state cancels any
	// existing scheduled fencing actions
	ValidAfter int64 `json:"validAfter,omitempty"`
}

type FencingRequestStatus struct {
	Complete     bool                         `json:"complete"`
	Result       string                       `json:"result"`
	Config       *string                      `json:"config,omitempty"`
	ActiveMethod *string                      `json:"activeMethod,omitempty"`
	ActiveJob    *string                      `json:"activeMethod,omitempty"`
	Updates      []FencingRequestStatusUpdate `json:"updates,omitempty"`
}

type FencingRequestStatusUpdate struct {
	Timestamp string `json:"timestamp"`
	Method    string `json:"method,omitempty"`
	Message   string `json:"message"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
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
	Spec              FencingConfig    `json:"spec"`
	Status            FencingSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FencingConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	NodeSelector      map[string]string `json:"nodeSelector,omitempty"`

	// The operator attempts sets in order until one succeeds All mechanisms
	// in a set are required to succeed in order for the set to succeed.
	//
	// A CLI tool/extension will be provided that allows an admin to create
	// FencingReques CRs and unfence one way operations like network and disk
	// based fencing events.
	Methods []FencingMethod `json:"methods"`
}

type FencingMethod struct {
	Name string `json:"name"`

	// If this method completes sucessfully, then recovery is considered complete
	StopOnSuccess bool `json:"stopOnSuccess"`

	// While StopOnSuccess controls when the fencing workflow reports sucess,
	// RequireAfterSeconds allows additional tasks to be scheduled at some
	// point afterwards.
	//
	// This allows the admin to configure workflows where the worker is put
	// into a safe but non-leathal state, allowing the root cause to be
	// triaged while container workloads recover - but also allows the worker
	// to automatically return to the pool through power fencing after
	// `RequireAfterSeconds`.
	//
	// Should the node return to a healthy state before `RequireAfterSeconds`
	// elapses, possibly through manual admin intervention, the additional
	// workflow step(s) will be cancelled.
	//
	// Should no other fencing method succeed, the method will be initiated
	// immediately.
	RequireAfterSeconds *int32 `json:"requireAfterSeconds,omitempty"`

	Retries    int32              `json:"retries"`
	Mechanisms []FencingMechanism `json:"mechanisms"`
}

type FencingMechanism struct {
	Container *v1.Container `json:"container"`

	ArgumentFormat string `json:"argumentFormat"`
	PassTargetAs   string `json:"passTargetAs,omitempty"`
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	Config        map[string]string      `json:"config"`
	DynamicConfig []FencingDynamicConfig `json:"dynamicConfig"`

	// An optional list of references to secrets in the same namespace
	// to use when calling fencing modules (ie. IPMI or cloud provider passwords).
	//
	// See https://kubernetes.io/docs/concepts/configuration/secret/ and/or
	// http://kubernetes.io/docs/user-guide/images#specifying-imagepullsecrets-on-a-pod
	//	Secrets map[string]v1.LocalObjectReference `json:"secrets"`
	Secrets map[string]string `json:"secrets"`
}

type FencingDynamicConfig struct {
	// If no default is supplied, an error will be logged and the
	// containing mechanism will be considered to have failed

	Field   string            `json:"field"`
	Default string            `json:"default"`
	Values  map[string]string `json:"values"`
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
