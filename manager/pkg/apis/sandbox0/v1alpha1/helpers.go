package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/sandbox0-ai/infra/manager/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func CheckTemplate(template *SandboxTemplate) error {
	if strings.Contains(template.Namespace, "-") ||
		strings.Contains(template.Name, "-") ||
		template.Spec.ClusterId != nil && strings.Contains(*template.Spec.ClusterId, "-") {
		return fmt.Errorf("namespace, name, and clusterId cannot contain hyphens")
	}
	rs := GenReplicasetName(template)
	if errs := validation.IsDNS1123Label(rs); len(errs) > 0 {
		return fmt.Errorf("generated id '%s' is invalid: %v", rs, errs)
	}
	// ReplicaSet name length must be limited to allow for the pod random suffix (usually 5 chars + hyphen).
	// Max DNS label length is 63. 63 - 1 (hyphen) - 5 (suffix) = 57.
	if len(rs) > 57 {
		return fmt.Errorf("generated id '%s' is too long (%d > 57). It must be <= 57 chars to accommodate K8s generated pod names", rs, len(rs))
	}
	return nil
}

func GenReplicasetName(template *SandboxTemplate) string {
	cfg := config.LoadConfig()
	var clusterId, namespace string
	if template.Spec.ClusterId != nil && *template.Spec.ClusterId != "" {
		clusterId = *template.Spec.ClusterId
	} else {
		clusterId = cfg.DefaultClusterId
	}
	if template.Namespace == "" {
		namespace = cfg.DefaultTemplateNamespace
	} else {
		namespace = template.Namespace
	}
	return fmt.Sprintf("%s-%s-%s", clusterId, namespace, template.Name)
}

// buildPodSpec builds a pod spec from a template
func BuildPodSpec(template *SandboxTemplate) corev1.PodSpec {
	spec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers:    buildContainers(template),
	}

	// Apply runtime class if specified
	if template.Spec.RuntimeClassName != nil {
		spec.RuntimeClassName = template.Spec.RuntimeClassName
	}

	// Apply pod-level overrides
	if template.Spec.Pod != nil {
		if template.Spec.Pod.NodeSelector != nil {
			spec.NodeSelector = template.Spec.Pod.NodeSelector
		}
		if template.Spec.Pod.ServiceAccountName != "" {
			spec.ServiceAccountName = template.Spec.Pod.ServiceAccountName
		}
	}
	return spec
}

// buildContainers builds containers from template
func buildContainers(template *SandboxTemplate) []corev1.Container {
	containers := []corev1.Container{
		buildContainer(&template.Spec.MainContainer, template, true),
	}

	for i := range template.Spec.Sidecars {
		containers = append(containers, template.Spec.Sidecars[i])
	}
	return containers
}

// buildContainer builds a single container
func buildContainer(spec *ContainerSpec, template *SandboxTemplate, isMain bool) corev1.Container {
	name := "procd"
	if !isMain {
		name = fmt.Sprintf("sidecar-%s", spec.Image)
	}

	container := corev1.Container{
		Name:            name,
		Image:           spec.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
	}

	if spec.ImagePullPolicy != "" {
		container.ImagePullPolicy = corev1.PullPolicy(spec.ImagePullPolicy)
	}

	// Environment variables
	var envVars []corev1.EnvVar
	for k, v := range template.Spec.EnvVars {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}
	for _, ev := range spec.Env {
		envVars = append(envVars, corev1.EnvVar{Name: ev.Name, Value: ev.Value})
	}
	container.Env = envVars

	// Security context
	if spec.SecurityContext != nil {
		container.SecurityContext = &corev1.SecurityContext{}
		if spec.SecurityContext.RunAsUser != nil {
			container.SecurityContext.RunAsUser = spec.SecurityContext.RunAsUser
		}
		if spec.SecurityContext.RunAsGroup != nil {
			container.SecurityContext.RunAsGroup = spec.SecurityContext.RunAsGroup
		}
		if spec.SecurityContext.Capabilities != nil {
			container.SecurityContext.Capabilities = &corev1.Capabilities{
				Drop: convertCapabilities(spec.SecurityContext.Capabilities.Drop),
			}
		}
	}

	return container
}

func convertCapabilities(caps []string) []corev1.Capability {
	if caps == nil {
		return nil
	}
	result := make([]corev1.Capability, len(caps))
	for i, cap := range caps {
		result[i] = corev1.Capability(cap)
	}
	return result
}

// BuildEgressSpec builds EgressPolicySpec from SandboxNetworkPolicy
func BuildEgressSpec(policy *TplSandboxNetworkPolicy) *EgressPolicySpec {
	if policy == nil {
		return &EgressPolicySpec{
			DefaultAction:     "deny",
			AlwaysDeniedCIDRs: PlatformDeniedCIDRs,
			EnforceProxyPorts: []int32{80, 443},
		}
	}

	spec := &EgressPolicySpec{
		AlwaysDeniedCIDRs: PlatformDeniedCIDRs,
		EnforceProxyPorts: []int32{80, 443},
	}

	switch policy.Mode {
	case NetworkModeAllowAll:
		spec.DefaultAction = "allow"
	case NetworkModeBlockAll:
		spec.DefaultAction = "deny"
	case NetworkModeCustom:
		spec.DefaultAction = "deny" // Custom defaults to deny
	default:
		spec.DefaultAction = "deny"
	}

	if policy.Egress != nil {
		spec.AllowedCIDRs = policy.Egress.AllowedIPs
		spec.DeniedCIDRs = policy.Egress.BlockedIPs
		spec.AllowedDomains = policy.Egress.AllowedDomains
		spec.DeniedDomains = policy.Egress.BlockedDomains
	}

	return spec
}

// BuildIngressSpec builds IngressPolicySpec from SandboxNetworkPolicy
func BuildIngressSpec(policy *TplSandboxNetworkPolicy) *IngressPolicySpec {
	spec := &IngressPolicySpec{
		DefaultAction: "deny", // Always default deny for ingress
		// Allow procd port from internal-gateway
		AllowedPorts: []PortSpec{
			{Port: 49983, Protocol: "tcp"},
		},
	}

	if policy != nil && policy.Ingress != nil {
		spec.AllowedSourceCIDRs = policy.Ingress.AllowedIPs
		spec.DeniedSourceCIDRs = policy.Ingress.BlockedIPs
	}

	return spec
}
