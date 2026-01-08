package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SandboxTemplate defines a template for creating sandboxes
type SandboxTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxTemplateSpec   `json:"spec"`
	Status SandboxTemplateStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SandboxTemplateList contains a list of SandboxTemplate
type SandboxTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxTemplate `json:"items"`
}

// SandboxTemplateSpec defines the desired state of SandboxTemplate
type SandboxTemplateSpec struct {
	Pod corev1.PodSpec `json:"pod,omitempty"`
	// Pool strategy
	Pool PoolStrategy `json:"pool,omitempty"`
}

// PoolStrategy defines pool strategy
type PoolStrategy struct {
	MinIdle int32 `json:"minIdle"` // Minimum idle pods (ReplicaSet replicas)
	MaxIdle int32 `json:"maxIdle"` // Maximum idle pods (enforced by CleanupController)
}

// SandboxTemplateStatus defines the observed state of SandboxTemplate
type SandboxTemplateStatus struct {
	// Pool statistics
	IdleCount   int32 `json:"idleCount"`
	ActiveCount int32 `json:"activeCount"`

	// Conditions
	Conditions []SandboxTemplateCondition `json:"conditions,omitempty"`

	// Last updated time
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
}

// SandboxTemplateCondition defines a condition of SandboxTemplate
type SandboxTemplateCondition struct {
	Type               SandboxTemplateConditionType `json:"type"`
	Status             ConditionStatus              `json:"status"`
	LastTransitionTime metav1.Time                  `json:"lastTransitionTime,omitempty"`
	Reason             string                       `json:"reason,omitempty"`
	Message            string                       `json:"message,omitempty"`
}

// SandboxTemplateConditionType defines condition type
type SandboxTemplateConditionType string

const (
	SandboxTemplateReady       SandboxTemplateConditionType = "Ready"
	SandboxTemplatePoolHealthy SandboxTemplateConditionType = "PoolHealthy"
)

// ConditionStatus defines condition status
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)
