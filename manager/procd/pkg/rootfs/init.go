//go:build linux
// +build linux

package rootfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"
)

// Config contains configuration for rootfs initialization
type Config struct {
	// SandboxID is the unique identifier for the sandbox
	SandboxID string

	// OverlayMountPath is where the overlay will be mounted
	OverlayMountPath string

	// LowerPath is the path to the base layer (read-only)
	LowerPath string

	// UpperPath is the path to the writable layer
	UpperPath string

	// WorkPath is the path to the overlay work directory
	WorkPath string

	// RootfsCWD is the working directory inside the rootfs
	RootfsCWD string

	// EnableChroot enables chroot execution
	EnableChroot bool
}

// Manager manages rootfs overlay for a sandbox
type Manager struct {
	config Config
	logger *logrus.Logger

	mounted bool
}

// NewManager creates a new rootfs manager
func NewManager(config Config, logger *logrus.Logger) *Manager {
	return &Manager{
		config: config,
		logger: logger,
	}
}

// Initialize prepares the rootfs overlay for use
func (m *Manager) Initialize(ctx context.Context) error {
	m.logger.WithFields(logrus.Fields{
		"sandbox_id":         m.config.SandboxID,
		"overlay_mount_path": m.config.OverlayMountPath,
		"lower_path":         m.config.LowerPath,
		"upper_path":         m.config.UpperPath,
	}).Info("Initializing rootfs overlay")

	// 1. Create mount point if it doesn't exist
	if err := os.MkdirAll(m.config.OverlayMountPath, 0755); err != nil {
		return fmt.Errorf("create mount point: %w", err)
	}

	// 2. Create upper and work directories if they don't exist
	if err := os.MkdirAll(m.config.UpperPath, 0755); err != nil {
		return fmt.Errorf("create upper directory: %w", err)
	}
	if err := os.MkdirAll(m.config.WorkPath, 0755); err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}

	// 3. Mount the overlay filesystem
	if err := m.mountOverlay(); err != nil {
		return fmt.Errorf("mount overlay: %w", err)
	}

	// 4. Prepare special filesystems (proc, dev, sys)
	if err := m.prepareSpecialFS(); err != nil {
		m.unmountOverlay()
		return fmt.Errorf("prepare special filesystems: %w", err)
	}

	m.mounted = true

	m.logger.WithField("sandbox_id", m.config.SandboxID).Info("Rootfs overlay initialized successfully")

	return nil
}

// mountOverlay mounts the overlay filesystem
func (m *Manager) mountOverlay() error {
	// Construct overlay mount options
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		m.config.LowerPath,
		m.config.UpperPath,
		m.config.WorkPath,
	)

	m.logger.WithFields(logrus.Fields{
		"mount_point": m.config.OverlayMountPath,
		"options":     options,
	}).Debug("Mounting overlay filesystem")

	// Mount overlay using syscall
	// Note: This requires CAP_SYS_ADMIN capability
	if err := syscall.Mount("overlay", m.config.OverlayMountPath, "overlay", 0, options); err != nil {
		return fmt.Errorf("mount overlay: %w", err)
	}

	return nil
}

// prepareSpecialFS prepares /proc, /dev, and /sys inside the chroot
func (m *Manager) prepareSpecialFS() error {
	rootfsPath := m.config.OverlayMountPath

	// Create /proc directory
	procPath := filepath.Join(rootfsPath, "proc")
	if err := os.MkdirAll(procPath, 0555); err != nil {
		m.logger.WithError(err).Warn("Failed to create /proc directory")
	}

	// Create /dev directory
	devPath := filepath.Join(rootfsPath, "dev")
	if err := os.MkdirAll(devPath, 0755); err != nil {
		m.logger.WithError(err).Warn("Failed to create /dev directory")
	}

	// Create /sys directory
	sysPath := filepath.Join(rootfsPath, "sys")
	if err := os.MkdirAll(sysPath, 0555); err != nil {
		m.logger.WithError(err).Warn("Failed to create /sys directory")
	}

	// Mount /proc
	if err := syscall.Mount("proc", procPath, "proc", 0, ""); err != nil {
		m.logger.WithError(err).Warn("Failed to mount /proc")
	}

	// Mount /dev as tmpfs (minimal device nodes)
	if err := syscall.Mount("tmpfs", devPath, "tmpfs", syscall.MS_NOSUID, "size=64k"); err != nil {
		m.logger.WithError(err).Warn("Failed to mount /dev tmpfs")
	}

	// Create minimal device nodes
	m.createMinimalDevices(devPath)

	// Mount /sys (read-only for security)
	if err := syscall.Mount("sysfs", sysPath, "sysfs", syscall.MS_RDONLY|syscall.MS_NOSUID, ""); err != nil {
		m.logger.WithError(err).Warn("Failed to mount /sys")
	}

	return nil
}

// createMinimalDevices creates minimal device nodes in /dev
func (m *Manager) createMinimalDevices(devPath string) {
	devices := []struct {
		name  string
		mode  uint32
		major uint32
		minor uint32
	}{
		{"null", syscall.S_IFCHR | 0666, 1, 3},
		{"zero", syscall.S_IFCHR | 0666, 1, 5},
		{"random", syscall.S_IFCHR | 0666, 1, 8},
		{"urandom", syscall.S_IFCHR | 0666, 1, 9},
		{"tty", syscall.S_IFCHR | 0666, 5, 0},
		{"ptmx", syscall.S_IFCHR | 0666, 5, 2},
	}

	for _, dev := range devices {
		path := filepath.Join(devPath, dev.name)
		devNum := int(dev.major<<8 | dev.minor)
		if err := syscall.Mknod(path, dev.mode, devNum); err != nil {
			m.logger.WithError(err).WithField("device", dev.name).Debug("Failed to create device node")
		}
	}

	// Create /dev/pts directory
	ptsPath := filepath.Join(devPath, "pts")
	if err := os.MkdirAll(ptsPath, 0755); err == nil {
		syscall.Mount("devpts", ptsPath, "devpts", 0, "")
	}

	// Create symlinks
	symlinks := []struct {
		link   string
		target string
	}{
		{"fd", "/proc/self/fd"},
		{"stdin", "/proc/self/fd/0"},
		{"stdout", "/proc/self/fd/1"},
		{"stderr", "/proc/self/fd/2"},
	}

	for _, sl := range symlinks {
		linkPath := filepath.Join(devPath, sl.link)
		if err := os.Symlink(sl.target, linkPath); err != nil {
			m.logger.WithError(err).WithField("link", sl.link).Debug("Failed to create symlink")
		}
	}
}

// GetChrootPath returns the path to use for chroot
func (m *Manager) GetChrootPath() string {
	return m.config.OverlayMountPath
}

// GetCWD returns the working directory inside the rootfs
func (m *Manager) GetCWD() string {
	if m.config.RootfsCWD != "" {
		return m.config.RootfsCWD
	}
	return "/"
}

// Cleanup unmounts and cleans up the rootfs overlay
func (m *Manager) Cleanup() error {
	if !m.mounted {
		return nil
	}

	// Unmount special filesystems
	m.unmountSpecialFS()

	// Unmount overlay
	if err := m.unmountOverlay(); err != nil {
		m.logger.WithError(err).Warn("Failed to unmount overlay")
		return err
	}

	m.mounted = false

	m.logger.WithField("sandbox_id", m.config.SandboxID).Info("Rootfs overlay cleaned up")

	return nil
}

// unmountSpecialFS unmounts special filesystems
func (m *Manager) unmountSpecialFS() {
	rootfsPath := m.config.OverlayMountPath

	// Unmount in reverse order
	paths := []string{
		filepath.Join(rootfsPath, "sys"),
		filepath.Join(rootfsPath, "dev", "pts"),
		filepath.Join(rootfsPath, "dev"),
		filepath.Join(rootfsPath, "proc"),
	}

	for _, path := range paths {
		if err := syscall.Unmount(path, syscall.MNT_DETACH); err != nil {
			m.logger.WithError(err).WithField("path", path).Debug("Failed to unmount")
		}
	}
}

// unmountOverlay unmounts the overlay filesystem
func (m *Manager) unmountOverlay() error {
	return syscall.Unmount(m.config.OverlayMountPath, syscall.MNT_DETACH)
}

// IsMounted returns whether the overlay is currently mounted
func (m *Manager) IsMounted() bool {
	return m.mounted
}

// PrepareExecCmd prepares command execution with chroot if enabled
func (m *Manager) PrepareExecCmd(cmdPath string, args []string, env []string, cwd string) (*syscall.ProcAttr, error) {
	if !m.config.EnableChroot || !m.mounted {
		return nil, nil
	}

	// Resolve absolute path in chroot
	absPath := cmdPath
	if cmdPath[0] != '/' {
		absPath = "/" + cmdPath
	}

	// Prepare working directory
	workDir := cwd
	if workDir == "" {
		workDir = m.GetCWD()
	}
	if workDir[0] != '/' {
		workDir = "/" + workDir
	}

	return &syscall.ProcAttr{
		Dir: workDir,
		Env: env,
		Files: []uintptr{
			uintptr(syscall.Stdin),
			uintptr(syscall.Stdout),
			uintptr(syscall.Stderr),
		},
		Sys: &syscall.SysProcAttr{
			Chroot:     m.config.OverlayMountPath,
			Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC,
		},
	}, nil
}
