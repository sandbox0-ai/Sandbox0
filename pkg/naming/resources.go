package naming

import (
	"fmt"
	"strings"
)

// TeamName returns a Kubernetes-safe name for a team resource.
func TeamName(teamID string) (string, error) {
	return slugWithHash(fmt.Sprintf("team-%s", teamID), dnsLabelMaxLen)
}

// UserName returns a Kubernetes-safe name for a user resource.
func UserName(userID string) (string, error) {
	return slugWithHash(fmt.Sprintf("user-%s", userID), dnsLabelMaxLen)
}

// ClusterName returns a Kubernetes-safe name for a cluster resource.
func ClusterName(clusterID string) (string, error) {
	key, err := encodeClusterID(clusterID)
	if err != nil {
		return "", err
	}
	return slugWithHash(fmt.Sprintf("cluster-%s", key), dnsLabelMaxLen)
}

// ValidateClusterName ensures cluster name is non-empty and safe for storage.
func ValidateClusterName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("cluster_name is required")
	}
	if len(trimmed) > 255 {
		return fmt.Errorf("cluster_name is too long (%d > 255)", len(trimmed))
	}
	if strings.Contains(trimmed, "/") {
		return fmt.Errorf("cluster_name cannot contain '/'")
	}
	return nil
}

// ClusterIDFromName generates a stable cluster_id from cluster_name.
func ClusterIDFromName(name string) (string, error) {
	if err := ValidateClusterName(name); err != nil {
		return "", err
	}
	return slugWithHash(name, clusterIDMaxLen)
}

// VolumeName returns a Kubernetes-safe name for a sandbox volume resource.
func VolumeName(teamID, volumeID string) (string, error) {
	return slugWithHash(fmt.Sprintf("vol-%s-%s", teamID, volumeID), dnsLabelMaxLen)
}

// SnapshotName returns a Kubernetes-safe name for a sandbox volume snapshot resource.
func SnapshotName(volumeID, snapshotID string) (string, error) {
	return slugWithHash(fmt.Sprintf("snap-%s-%s", volumeID, snapshotID), dnsLabelMaxLen)
}
