package framework

import (
	"context"
	"fmt"
)

// Cluster manages a Kind-backed Kubernetes cluster for E2E tests.
type Cluster struct {
	Name       string
	Kubeconfig string
}

// NewCluster creates a new cluster descriptor.
func NewCluster(name string) *Cluster {
	return &Cluster{
		Name: name,
	}
}

// CreateKind creates a Kind cluster with the provided config file.
func (c *Cluster) CreateKind(ctx context.Context, configPath string) error {
	if c == nil {
		return fmt.Errorf("cluster is nil")
	}

	args := []string{"create", "cluster", "--name", c.Name}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}

	return RunCommand(ctx, "kind", args...)
}

// DeleteKind deletes the Kind cluster.
func (c *Cluster) DeleteKind(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("cluster is nil")
	}

	return RunCommand(ctx, "kind", "delete", "cluster", "--name", c.Name)
}

// LoadDockerImage loads a local Docker image into the cluster.
func (c *Cluster) LoadDockerImage(ctx context.Context, image string) error {
	if c == nil {
		return fmt.Errorf("cluster is nil")
	}
	if image == "" {
		return fmt.Errorf("image is required")
	}

	return RunCommand(ctx, "kind", "load", "docker-image", image, "--name", c.Name)
}
