package naming

import (
	"fmt"
	"strings"
)

// This file centralizes naming conventions for:
// - S3/object-store key prefixes (used by JuiceFS chunk/object storage)
// - JuiceFS internal filesystem paths (used by JuiceFS meta operations)
//
// These are different namespaces and do not need to be identical, but they must
// be consistent within their own layers to avoid data loss or cross-tenant leaks.

func validateID(kind, id string) error {
	if id == "" {
		return fmt.Errorf("%s is empty", kind)
	}
	// Disallow path separators to avoid path traversal / prefix injection.
	if strings.Contains(id, "/") {
		return fmt.Errorf("%s contains invalid '/'", kind)
	}
	return nil
}

// S3VolumePrefix returns the object-store prefix used for a team's volume data.
// Example: sandboxvolumes/<teamID>/<volumeID>
func S3VolumePrefix(teamID, volumeID string) (string, error) {
	if err := validateID("teamID", teamID); err != nil {
		return "", err
	}
	if err := validateID("volumeID", volumeID); err != nil {
		return "", err
	}
	return fmt.Sprintf("sandboxvolumes/%s/%s", teamID, volumeID), nil
}

// JuiceFSVolumePath returns the internal JuiceFS directory where a volume lives.
// Example: /volumes/<volumeID>
func JuiceFSVolumePath(volumeID string) (string, error) {
	if err := validateID("volumeID", volumeID); err != nil {
		return "", err
	}
	return fmt.Sprintf("/volumes/%s", volumeID), nil
}

// JuiceFSSnapshotParentPath returns the parent directory for snapshots of a volume.
// Example: /snapshots/<volumeID>
func JuiceFSSnapshotParentPath(volumeID string) (string, error) {
	if err := validateID("volumeID", volumeID); err != nil {
		return "", err
	}
	return fmt.Sprintf("/snapshots/%s", volumeID), nil
}

// JuiceFSSnapshotPath returns the internal JuiceFS path for a specific snapshot.
// Example: /snapshots/<volumeID>/<snapshotID>
func JuiceFSSnapshotPath(volumeID, snapshotID string) (string, error) {
	if err := validateID("volumeID", volumeID); err != nil {
		return "", err
	}
	if err := validateID("snapshotID", snapshotID); err != nil {
		return "", err
	}
	return fmt.Sprintf("/snapshots/%s/%s", volumeID, snapshotID), nil
}
