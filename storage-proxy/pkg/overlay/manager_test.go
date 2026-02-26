package overlay

import (
	"testing"

	"github.com/sandbox0-ai/infra/storage-proxy/pkg/pathutil"
	"github.com/stretchr/testify/assert"
)

// ========================================
// GenerateSnapshotID Tests
// ========================================

func TestGenerateSnapshotID(t *testing.T) {
	id1 := GenerateSnapshotID()
	id2 := GenerateSnapshotID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
	assert.Contains(t, id1, "rs-")
	assert.Len(t, id1, 11) // "rs-" + 8 chars = 11
}

// ========================================
// SplitPath Tests
// ========================================

func TestSplitPath_Overlay(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "empty path",
			path:     "",
			expected: nil,
		},
		{
			name:     "single component",
			path:     "/a",
			expected: []string{"a"},
		},
		{
			name:     "multiple components",
			path:     "/a/b/c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "no leading slash",
			path:     "a/b/c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "double slashes",
			path:     "/a//b///c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "trailing slash",
			path:     "/a/b/c/",
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pathutil.SplitPath(tt.path)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// ========================================
// Interface Verification Tests
// ========================================

func TestManager_ImplementsOverlayManager(t *testing.T) {
	// This test verifies that *Manager implements the OverlayManager interface
	// If this compiles, the interface is correctly implemented
	var _ OverlayManager = (*Manager)(nil)
}

// ========================================
// OverlayContext Tests
// ========================================

func TestOverlayContext_Fields(t *testing.T) {
	ctx := &OverlayContext{
		SandboxID:      "sandbox-1",
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperVolumeID:  "vol-1",
		LowerPath:      "/lower",
		UpperPath:      "/upper",
		WorkPath:       "/work",
		Mounted:        true,
		UpperRootInode: 100,
	}

	assert.Equal(t, "sandbox-1", ctx.SandboxID)
	assert.Equal(t, "team-1", ctx.TeamID)
	assert.Equal(t, "layer-1", ctx.BaseLayerID)
	assert.Equal(t, "vol-1", ctx.UpperVolumeID)
	assert.Equal(t, "/lower", ctx.LowerPath)
	assert.Equal(t, "/upper", ctx.UpperPath)
	assert.Equal(t, "/work", ctx.WorkPath)
	assert.True(t, ctx.Mounted)
	assert.Equal(t, uint64(100), uint64(ctx.UpperRootInode))
}

// ========================================
// CreateOverlayConfig Tests
// ========================================

func TestCreateOverlayConfig_Fields(t *testing.T) {
	cfg := &CreateOverlayConfig{
		SandboxID:    "sandbox-1",
		TeamID:       "team-1",
		BaseLayerID:  "layer-1",
		FromSnapshot: "snapshot-1",
	}

	assert.Equal(t, "sandbox-1", cfg.SandboxID)
	assert.Equal(t, "team-1", cfg.TeamID)
	assert.Equal(t, "layer-1", cfg.BaseLayerID)
	assert.Equal(t, "snapshot-1", cfg.FromSnapshot)
}

// ========================================
// MountInfo Tests
// ========================================

func TestMountInfo_Fields(t *testing.T) {
	info := &MountInfo{
		SandboxID:  "sandbox-1",
		LowerPath:  "/lower",
		UpperPath:  "/upper",
		WorkPath:   "/work",
		UpperInode: 100,
	}

	assert.Equal(t, "sandbox-1", info.SandboxID)
	assert.Equal(t, "/lower", info.LowerPath)
	assert.Equal(t, "/upper", info.UpperPath)
	assert.Equal(t, "/work", info.WorkPath)
	assert.Equal(t, uint64(100), uint64(info.UpperInode))
}

// ========================================
// Error Tests
// ========================================

func TestErrors(t *testing.T) {
	// Verify error constants are defined and have expected values
	assert.EqualError(t, ErrOverlayNotFound, "overlay not found")
	assert.EqualError(t, ErrLayerNotReady, "base layer not ready")
	assert.EqualError(t, ErrVolumeNotMounted, "volume not mounted")
}
