//go:build !linux
// +build !linux

package rootfs

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
)

// Config contains configuration for rootfs initialization
type Config struct {
	SandboxID        string
	OverlayMountPath string
	LowerPath        string
	UpperPath        string
	WorkPath         string
	RootfsCWD        string
	EnableChroot     bool
}

// Manager manages rootfs overlay for a sandbox (stub for non-Linux)
type Manager struct {
	config  Config
	logger  *logrus.Logger
	mounted bool
}

// NewManager creates a new rootfs manager
func NewManager(config Config, logger *logrus.Logger) *Manager {
	return &Manager{
		config: config,
		logger: logger,
	}
}

// Initialize prepares the rootfs overlay for use (stub)
func (m *Manager) Initialize(ctx context.Context) error {
	m.logger.Warn("Rootfs overlay is only supported on Linux")
	return fmt.Errorf("rootfs overlay not supported on this platform")
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

// Cleanup unmounts and cleans up the rootfs overlay (stub)
func (m *Manager) Cleanup() error {
	m.mounted = false
	return nil
}

// IsMounted returns whether the overlay is currently mounted
func (m *Manager) IsMounted() bool {
	return m.mounted
}

// PrepareExecCmd prepares command execution (stub for non-Linux)
func (m *Manager) PrepareExecCmd(cmdPath string, args []string, env []string, cwd string) (any, error) {
	return nil, fmt.Errorf("chroot not supported on this platform")
}
