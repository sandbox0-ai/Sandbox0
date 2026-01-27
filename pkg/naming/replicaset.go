package naming

import (
	"fmt"
	"strings"
)

// ProcdConfigSecretName generates a Kubernetes-safe Secret name for procd config.
// Format: procd-secret-<clusterKey>-<templateKey>
func ProcdConfigSecretName(clusterID, templateName string) (string, error) {
	clusterKey, err := encodeClusterID(clusterID)
	if err != nil {
		return "", err
	}
	base := fmt.Sprintf("procd-secret-%s-%s", clusterKey, templateName)
	return slugWithHash(base, dnsLabelMaxLen)
}

// ReplicasetName generates a Kubernetes-safe ReplicaSet name.
// Format: rs-<clusterKey>-<templateKey>
func ReplicasetName(clusterID, templateName string) (string, error) {
	clusterKey, err := encodeClusterID(clusterID)
	if err != nil {
		return "", err
	}
	prefix := fmt.Sprintf("%s-%s-", sandboxNamePrefix, clusterKey)
	remaining := replicaSetMaxLen - len(prefix)
	if remaining <= 0 {
		return "", fmt.Errorf("cluster key too long to build replicaset name")
	}
	templateKey, err := slugWithHash(templateName, remaining)
	if err != nil {
		return "", err
	}
	name := prefix + templateKey
	if err := validateDNSLabel(name); err != nil {
		return "", err
	}
	return name, nil
}

// SandboxName generates a sandbox (pod) name using the ReplicaSet name and a random suffix.
func SandboxName(clusterID, templateName, randSuffix string) (string, error) {
	if randSuffix == "" {
		return "", fmt.Errorf("randSuffix is empty")
	}
	if strings.Contains(randSuffix, "-") {
		return "", fmt.Errorf("randSuffix cannot contain hyphens")
	}
	if len(randSuffix) > podRandSuffixLen {
		return "", fmt.Errorf("randSuffix is too long (%d > %d)", len(randSuffix), podRandSuffixLen)
	}
	rsName, err := ReplicasetName(clusterID, templateName)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s", rsName, randSuffix)
	if err := validateDNSLabel(name); err != nil {
		return "", err
	}
	return name, nil
}

// CheckTemplateName validates template naming constraints for K8s resources.
func CheckTemplateName(clusterID, templateName string) error {
	_, err := ReplicasetName(clusterID, templateName)
	return err
}
