// Package pathutil provides utilities for path manipulation in JuiceFS operations.
package pathutil

// SplitPath splits a path into its components, removing empty parts.
// For example, "/a/b/c" returns ["a", "b", "c"].
// Empty paths return nil.
func SplitPath(path string) []string {
	if path == "" {
		return nil
	}
	result := []string{}
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if i > start {
				result = append(result, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		result = append(result, path[start:])
	}
	return result
}

// JoinPath joins path components into a single path.
// The result always starts without a leading slash.
func JoinPath(components []string) string {
	if len(components) == 0 {
		return ""
	}
	result := ""
	for i, comp := range components {
		if comp == "" {
			continue
		}
		if i > 0 && result != "" {
			result += "/"
		}
		result += comp
	}
	return result
}

// ParentPath returns the parent directory path for a given path.
// For example, "/a/b/c" returns "/a/b".
// If the path has no parent, it returns "".
func ParentPath(path string) string {
	components := SplitPath(path)
	if len(components) <= 1 {
		return ""
	}
	return JoinPath(components[:len(components)-1])
}

// BaseName returns the last component of a path.
// For example, "/a/b/c" returns "c".
// If the path is empty, it returns "".
func BaseName(path string) string {
	components := SplitPath(path)
	if len(components) == 0 {
		return ""
	}
	return components[len(components)-1]
}
