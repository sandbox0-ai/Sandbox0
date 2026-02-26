package rootfs

import (
	"fmt"
	"syscall"

	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/pathutil"
)

var (
	// ErrPathNotFound indicates that the requested path was not found in JuiceFS
	ErrPathNotFound = fmt.Errorf("path not found")
)

// PathNavigator provides utilities for navigating and manipulating paths in JuiceFS
type PathNavigator struct {
	metaClient metaClient
}

// NewPathNavigator creates a new path navigator
func NewPathNavigator(metaClient meta.Meta) *PathNavigator {
	return &PathNavigator{
		metaClient: metaClient,
	}
}

// PathInfo contains information about a path in JuiceFS
type PathInfo struct {
	ParentIno meta.Ino
	Inode     meta.Ino
	Name      string
	Attr      *meta.Attr
}

// NavigatePath navigates to a path and returns the path information
func (p *PathNavigator) NavigatePath(jfsCtx meta.Context, path string) (*PathInfo, error) {
	components := pathutil.SplitPath(path)
	if len(components) == 0 {
		return nil, ErrPathNotFound
	}

	current := meta.RootInode
	parentIno := meta.RootInode
	var attr meta.Attr

	for i, component := range components {
		if component == "" {
			continue
		}

		var next meta.Ino
		errno := p.metaClient.Lookup(jfsCtx, current, component, &next, &attr, false)
		if errno != 0 {
			if errno == syscall.ENOENT {
				return nil, fmt.Errorf("%w: %s", ErrPathNotFound, path)
			}
			return nil, fmt.Errorf("lookup %s: %w", component, errno)
		}

		// Track parent before moving to next
		if i < len(components)-1 {
			parentIno = next
		}
		current = next
	}

	return &PathInfo{
		ParentIno: parentIno,
		Inode:     current,
		Name:      components[len(components)-1],
		Attr:      &attr,
	}, nil
}

// ClearDirectory removes all contents of a directory without removing the directory itself
func (p *PathNavigator) ClearDirectory(jfsCtx meta.Context, dirIno meta.Ino) error {
	var entries []*meta.Entry
	errno := p.metaClient.Readdir(jfsCtx, dirIno, 0, &entries)
	if errno != 0 {
		return fmt.Errorf("readdir: %w", errno)
	}

	for _, entry := range entries {
		name := string(entry.Name)
		if name == "." || name == ".." {
			continue
		}

		var removeCount uint64
		// Use recursive removal (true) with 4 threads
		errno := p.metaClient.Remove(jfsCtx, dirIno, name, true, 4, &removeCount)
		if errno != 0 && errno != syscall.ENOENT {
			// Continue removing other entries even if one fails
			continue
		}
	}

	return nil
}

// ClonePath clones a source path to a destination path using JuiceFS Clone (COW)
// Returns the count of cloned files and total size
func (p *PathNavigator) ClonePath(jfsCtx meta.Context, srcPath, dstPath string) (count, total uint64, err error) {
	// Get source path info
	srcInfo, err := p.NavigatePath(jfsCtx, srcPath)
	if err != nil {
		return 0, 0, fmt.Errorf("navigate source path: %w", err)
	}

	// Get destination parent path
	dstComponents := pathutil.SplitPath(dstPath)
	if len(dstComponents) == 0 {
		return 0, 0, fmt.Errorf("invalid destination path: %s", dstPath)
	}

	// Ensure destination parent directory exists
	dstParentPath := dstComponents[:len(dstComponents)-1]
	dstParentIno := meta.RootInode
	var attr meta.Attr

	for _, component := range dstParentPath {
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := p.metaClient.Lookup(jfsCtx, dstParentIno, component, &next, &attr, false)
		if errno != 0 {
			errno = p.metaClient.Mkdir(jfsCtx, dstParentIno, component, 0755, 0, 0, &next, &attr)
			if errno != 0 {
				return 0, 0, fmt.Errorf("mkdir %s: %w", component, errno)
			}
		}
		dstParentIno = next
	}

	dstName := dstComponents[len(dstComponents)-1]

	// Clone using JuiceFS clone (COW)
	errno := p.metaClient.Clone(jfsCtx, srcInfo.ParentIno, srcInfo.Inode, dstParentIno, dstName, 0, 0, &count, &total)
	if errno != 0 {
		return 0, 0, fmt.Errorf("clone failed: %w", errno)
	}

	return count, total, nil
}

// RemovePath removes a path from JuiceFS
func (p *PathNavigator) RemovePath(jfsCtx meta.Context, path string) error {
	components := pathutil.SplitPath(path)
	if len(components) == 0 {
		return nil
	}

	// Navigate to parent
	parent := meta.RootInode
	var attr meta.Attr

	for i := range len(components) - 1 {
		component := components[i]
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := p.metaClient.Lookup(jfsCtx, parent, component, &next, &attr, false)
		if errno != 0 {
			return nil // Parent doesn't exist, nothing to remove
		}
		parent = next
	}

	// Remove the entry
	name := components[len(components)-1]
	var removeCount uint64
	errno := p.metaClient.Remove(jfsCtx, parent, name, true, 4, &removeCount)
	if errno != 0 && errno != syscall.ENOENT {
		return fmt.Errorf("remove %s: %w", name, errno)
	}

	return nil
}

// NavigateToParent navigates to the parent directory of a path and returns parent info
func (p *PathNavigator) NavigateToParent(jfsCtx meta.Context, path string) (parentIno meta.Ino, name string, err error) {
	components := pathutil.SplitPath(path)
	if len(components) == 0 {
		return 0, "", ErrPathNotFound
	}

	parentIno = meta.RootInode
	var attr meta.Attr

	for i := range len(components) - 1 {
		component := components[i]
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := p.metaClient.Lookup(jfsCtx, parentIno, component, &next, &attr, false)
		if errno != 0 {
			return 0, "", fmt.Errorf("parent path not found: %s", path)
		}
		parentIno = next
	}

	name = components[len(components)-1]
	return parentIno, name, nil
}

// Rename renames a path (atomic operation in JuiceFS)
func (p *PathNavigator) Rename(jfsCtx meta.Context, srcPath, dstPath string) error {
	// Get source parent and name
	srcParent, srcName, err := p.NavigateToParent(jfsCtx, srcPath)
	if err != nil {
		return fmt.Errorf("navigate source parent: %w", err)
	}

	// Get destination parent and name
	dstParent, dstName, err := p.NavigateToParent(jfsCtx, dstPath)
	if err != nil {
		return fmt.Errorf("navigate destination parent: %w", err)
	}

	// Get source inode
	var srcIno meta.Ino
	var attr meta.Attr
	errno := p.metaClient.Lookup(jfsCtx, srcParent, srcName, &srcIno, &attr, false)
	if errno != 0 {
		if errno == syscall.ENOENT {
			return fmt.Errorf("%w: %s", ErrPathNotFound, srcPath)
		}
		return fmt.Errorf("lookup source: %w", errno)
	}

	// Perform rename using JuiceFS Rename
	// Note: JuiceFS meta.Rename signature is:
	// Rename(ctx Context, parentSrc meta.Ino, nameSrc string, parentDst meta.Ino, nameDst string, flags uint32, inode *meta.Ino, attr *meta.Attr) syscall.Errno
	errno = p.metaClient.Rename(jfsCtx, srcParent, srcName, dstParent, dstName, 0, &srcIno, &attr)
	if errno != 0 {
		return fmt.Errorf("rename %s to %s: %w", srcPath, dstPath, errno)
	}

	return nil
}
