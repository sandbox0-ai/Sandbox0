package pathutil

import (
	"reflect"
	"testing"
)

func TestSplitPath(t *testing.T) {
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
			name:     "root path",
			path:     "/",
			expected: []string{}, // "/" splits to empty slice (no components)
		},
		{
			name:     "single component",
			path:     "a",
			expected: []string{"a"},
		},
		{
			name:     "single component with leading slash",
			path:     "/a",
			expected: []string{"a"},
		},
		{
			name:     "multiple components",
			path:     "/a/b/c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "multiple components without leading slash",
			path:     "a/b/c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "trailing slash",
			path:     "/a/b/c/",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "double slashes",
			path:     "/a//b///c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "complex path",
			path:     "/rootfs/sandbox-123/upper",
			expected: []string{"rootfs", "sandbox-123", "upper"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitPath(tt.path)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("SplitPath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestJoinPath(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		expected   string
	}{
		{
			name:       "empty components",
			components: []string{},
			expected:   "",
		},
		{
			name:       "nil components",
			components: nil,
			expected:   "",
		},
		{
			name:       "single component",
			components: []string{"a"},
			expected:   "a",
		},
		{
			name:       "multiple components",
			components: []string{"a", "b", "c"},
			expected:   "a/b/c",
		},
		{
			name:       "components with empty strings",
			components: []string{"a", "", "b", "", "c"},
			expected:   "a/b/c",
		},
		{
			name:       "all empty strings",
			components: []string{"", "", ""},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.components)
			if result != tt.expected {
				t.Errorf("JoinPath(%v) = %q, want %q", tt.components, result, tt.expected)
			}
		})
	}
}

func TestParentPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "single component",
			path:     "a",
			expected: "",
		},
		{
			name:     "two components",
			path:     "/a/b",
			expected: "a",
		},
		{
			name:     "multiple components",
			path:     "/a/b/c",
			expected: "a/b",
		},
		{
			name:     "deep path",
			path:     "/rootfs/sandbox-123/snapshots/rs-abc",
			expected: "rootfs/sandbox-123/snapshots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParentPath(tt.path)
			if result != tt.expected {
				t.Errorf("ParentPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestBaseName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "single component",
			path:     "a",
			expected: "a",
		},
		{
			name:     "multiple components",
			path:     "/a/b/c",
			expected: "c",
		},
		{
			name:     "trailing slash",
			path:     "/a/b/c/",
			expected: "c",
		},
		{
			name:     "rootfs path",
			path:     "/rootfs/sandbox-123/upper",
			expected: "upper",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BaseName(tt.path)
			if result != tt.expected {
				t.Errorf("BaseName(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestSplitPathAndJoinRoundTrip(t *testing.T) {
	tests := []string{
		"/a/b/c",
		"/rootfs/sandbox-123/upper",
		"/layers/team-abc/layer-xyz",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			components := SplitPath(path)
			result := JoinPath(components)
			// Note: JoinPath doesn't add leading slash
			expected := path
			if len(expected) > 0 && expected[0] == '/' {
				expected = expected[1:]
			}
			if result != expected {
				t.Errorf("Round trip failed: SplitPath(%q) -> JoinPath = %q, want %q", path, result, expected)
			}
		})
	}
}
