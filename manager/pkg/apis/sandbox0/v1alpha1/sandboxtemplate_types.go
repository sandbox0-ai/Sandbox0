package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	// Description of the template
	Description string   `json:"description,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Tags        []string `json:"tags,omitempty"`

	// MainContainer configuration (required)
	MainContainer ContainerSpec `json:"mainContainer"`

	// Sidecar containers (optional)
	Sidecars []corev1.Container `json:"sidecars,omitempty"`

	// Pod-level configuration
	Pod *PodSpecOverride `json:"pod,omitempty"`

	// Template Sandbox Network policy (template-level default)
	Network *TplSandboxNetworkPolicy `json:"network,omitempty"`

	// Pool strategy
	Pool PoolStrategy `json:"pool"`

	// Lifecycle management
	Lifecycle *LifecyclePolicy `json:"lifecycle,omitempty"`

	// Environment variables (global, shared by all containers)
	EnvVars map[string]string `json:"envVars,omitempty"`

	// Access control
	Public       bool     `json:"public,omitempty"`
	AllowedTeams []string `json:"allowedTeams,omitempty"`

	// Environment configuration
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`
	ClusterId        *string `json:"clusterId,omitempty"`

	// Rootfs configuration for overlay-based container rootfs
	// When enabled, the sandbox uses a FUSE overlay filesystem with:
	// - lowerdir: base layer (from BaseLayerID, extracted to JuiceFS)
	// - upperdir: sandbox writable layer (each sandbox has its own)
	// This enables rootfs-level snapshot/restore/fork capabilities.
	Rootfs *RootfsConfig `json:"rootfs,omitempty"`

	// BaseLayerID is managed by the system. Users should not set this field.
	// The base layer contains the container image filesystem extracted to JuiceFS
	// and is shared across sandboxes. This field is automatically populated based
	// on spec.mainContainer.image when spec.rootfs.enabled is true.
	// +kubebuilder:validation:Optional
	BaseLayerID string `json:"-"`
}

// SystemTeamID is the team ID used for public template baselayers
const SystemTeamID = "_system_"

// RootfsConfig defines the rootfs overlay configuration
type RootfsConfig struct {
	// Enabled enables rootfs overlay for this template
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// SnapshotPolicy configures automatic snapshot behavior
	SnapshotPolicy *RootfsSnapshotPolicy `json:"snapshotPolicy,omitempty"`

	// PersistentRW enables persistent writable layer across sandbox lifecycles.
	// When true, the upperdir is preserved between sandbox restarts.
	// When false (default), upperdir is created fresh for each sandbox instance.
	PersistentRW bool `json:"persistentRW,omitempty"`
}

// RootfsSnapshotPolicy defines automatic snapshot behavior
type RootfsSnapshotPolicy struct {
	// Enabled enables automatic snapshots
	Enabled bool `json:"enabled"`

	// MaxCount limits the number of automatic snapshots to retain
	MaxCount int `json:"maxCount,omitempty"`

	// Retention defines how long to keep snapshots (e.g., "24h")
	Retention string `json:"retention,omitempty"`
}

type ContainerSpec struct {
	// Image is the container image reference.
	// When spec.baseLayerId is specified, this field is ignored as the base layer
	// provides the rootfs. When baseLayerId is not specified, this image is used
	// for on-demand extraction (fallback mode).
	Image string `json:"image,omitempty"`

	ImagePullPolicy string           `json:"imagePullPolicy,omitempty"`
	Env             []EnvVar         `json:"env,omitempty"`
	Resources       ResourceQuota    `json:"resources"`
	SecurityContext *SecurityContext `json:"securityContext,omitempty"`

	// RootfsCwd specifies the working directory relative to the rootfs overlay.
	// Only used when rootfs is enabled. Overrides any WORKDIR from the image.
	RootfsCwd string `json:"rootfsCwd,omitempty"`
}

// EnvVar represents an environment variable
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ResourceRequirements defines resource requirements for containers
type ResourceRequirements struct {
	Limits   map[string]string `json:"limits,omitempty"`   // e.g. {"cpu": "2", "memory": "4Gi"}
	Requests map[string]string `json:"requests,omitempty"` // e.g. {"cpu": "1", "memory": "2Gi"}
}

// SecurityContext defines security context for containers
type SecurityContext struct {
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	RunAsUser    *int64        `json:"runAsUser,omitempty"`
	RunAsGroup   *int64        `json:"runAsGroup,omitempty"`
}

// Capabilities defines Linux capabilities
type Capabilities struct {
	// Add field is removed to prevent privilege escalation
	Drop []string `json:"drop,omitempty"` // e.g. ["NET_RAW"]
}

// PodSpecOverride allows overriding pod-level settings
type PodSpecOverride struct {
	NodeSelector       map[string]string `json:"nodeSelector,omitempty"`
	Affinity           *Affinity         `json:"affinity,omitempty"`
	Tolerations        []Toleration      `json:"tolerations,omitempty"`
	ServiceAccountName string            `json:"serviceAccountName,omitempty"`
}

// Affinity defines pod affinity rules
type Affinity struct {
	NodeAffinity *NodeAffinity `json:"nodeAffinity,omitempty"`
	PodAffinity  *PodAffinity  `json:"podAffinity,omitempty"`
}

// NodeAffinity defines node affinity rules
type NodeAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  *NodeSelector             `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []PreferredSchedulingTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// NodeSelector defines node selector
type NodeSelector struct {
	NodeSelectorTerms []NodeSelectorTerm `json:"nodeSelectorTerms"`
}

// NodeSelectorTerm defines node selector term
type NodeSelectorTerm struct {
	MatchExpressions []NodeSelectorRequirement `json:"matchExpressions,omitempty"`
	MatchFields      []NodeSelectorRequirement `json:"matchFields,omitempty"`
}

// NodeSelectorRequirement defines node selector requirement
type NodeSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// PreferredSchedulingTerm defines preferred scheduling term
type PreferredSchedulingTerm struct {
	Weight     int32            `json:"weight"`
	Preference NodeSelectorTerm `json:"preference"`
}

// PodAffinity defines pod affinity rules
type PodAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []PodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// PodAffinityTerm defines pod affinity term
type PodAffinityTerm struct {
	LabelSelector *LabelSelector `json:"labelSelector,omitempty"`
	Namespaces    []string       `json:"namespaces,omitempty"`
	TopologyKey   string         `json:"topologyKey"`
}

// LabelSelector defines label selector
type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// LabelSelectorRequirement defines label selector requirement
type LabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// WeightedPodAffinityTerm defines weighted pod affinity term
type WeightedPodAffinityTerm struct {
	Weight          int32           `json:"weight"`
	PodAffinityTerm PodAffinityTerm `json:"podAffinityTerm"`
}

// Toleration defines pod toleration
type Toleration struct {
	Key      string `json:"key,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
	Effect   string `json:"effect,omitempty"`
}

// ResourceQuota defines resource quota (per template)
type ResourceQuota struct {
	CPU    resource.Quantity `json:"cpu,omitempty"`    // e.g. "2"
	Memory resource.Quantity `json:"memory,omitempty"` // e.g. "4Gi"
}

// PoolStrategy defines pool strategy
type PoolStrategy struct {
	MinIdle   int32 `json:"minIdle"` // Minimum idle pods (ReplicaSet replicas)
	MaxIdle   int32 `json:"maxIdle"` // Maximum idle pods (enforced by CleanupController)
	AutoScale bool  `json:"autoScale"`
}

// TplSandboxNetworkPolicy defines network policy (template-level default)
type TplSandboxNetworkPolicy struct {
	Mode   NetworkPolicyMode    `json:"mode"`
	Egress *NetworkEgressPolicy `json:"egress,omitempty"`
}

// NetworkPolicyMode defines network policy mode
type NetworkPolicyMode string

const (
	NetworkModeAllowAll NetworkPolicyMode = "allow-all"
	NetworkModeBlockAll NetworkPolicyMode = "block-all"
)

// NetworkEgressPolicy defines egress policy
type NetworkEgressPolicy struct {
	AllowedCIDRs   []string   `json:"allowedCidrs,omitempty"`
	AllowedDomains []string   `json:"allowedDomains,omitempty"`
	DeniedCIDRs    []string   `json:"deniedCidrs,omitempty"`
	DeniedDomains  []string   `json:"deniedDomains,omitempty"`
	AllowedPorts   []PortSpec `json:"allowedPorts,omitempty"`
	DeniedPorts    []PortSpec `json:"deniedPorts,omitempty"`
}

// LifecyclePolicy defines lifecycle policy
type LifecyclePolicy struct {
	DefaultTTL  int32 `json:"defaultTTL,omitempty"`  // Default TTL in seconds
	MaxTTL      int32 `json:"maxTTL,omitempty"`      // Maximum TTL in seconds
	IdleTimeout int32 `json:"idleTimeout,omitempty"` // Idle timeout in seconds
	// use pure k8s hooks
	PreStop *PreStopHook `json:"preStop,omitempty"` // PreStop hook
}

// PreStopHook defines pre-stop hook
type PreStopHook struct {
	Command        []string `json:"command,omitempty"`
	TimeoutSeconds int32    `json:"timeoutSeconds,omitempty"`
}

// SandboxTemplateStatus defines the observed state of SandboxTemplate
type SandboxTemplateStatus struct {
	// Pool statistics
	IdleCount   int32 `json:"idleCount"`
	ActiveCount int32 `json:"activeCount"`

	// Baselayer status (read-only, visible to users)
	// BaseLayerID is the system-managed base layer ID (read-only)
	BaseLayerID string `json:"baseLayerId,omitempty"`
	// BaseLayerStatus indicates the baselayer extraction status: pending, extracting, ready, failed
	BaseLayerStatus string `json:"baseLayerStatus,omitempty"`
	// BaseLayerError contains the error message if baselayer extraction failed
	BaseLayerError string `json:"baseLayerError,omitempty"`

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
	SandboxTemplateReady          SandboxTemplateConditionType = "Ready"
	SandboxTemplatePoolHealthy    SandboxTemplateConditionType = "PoolHealthy"
	SandboxTemplateBaseLayerReady SandboxTemplateConditionType = "BaseLayerReady"

	// TemplateFinalizer is used to clean up baselayer references on deletion
	TemplateFinalizer = "sandbox0.ai/baselayer-cleanup"
)

// BaseLayer status constants
const (
	BaseLayerStatusPending    = "pending"
	BaseLayerStatusExtracting = "extracting"
	BaseLayerStatusReady      = "ready"
	BaseLayerStatusFailed     = "failed"
)

// ConditionStatus defines condition status
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)
