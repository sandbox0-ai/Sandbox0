package framework

import (
	"context"
	"fmt"
	"os"
	"strings"
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
// If the cluster already exists, it does nothing and returns nil.
func (c *Cluster) CreateKind(ctx context.Context, configPath string) error {
	if c == nil {
		return fmt.Errorf("cluster is nil")
	}

	fmt.Printf("Checking if Kind cluster %q exists...\n", c.Name)
	// Check if cluster exists
	err := RunCommand(ctx, "kind", "get", "clusters")
	if err == nil {
		output, err := RunCommandOutput(ctx, "kind", "get", "clusters")
		if err == nil && strings.Contains(output, c.Name) {
			fmt.Printf("Kind cluster %q already exists, skipping creation.\n", c.Name)
			return nil
		}
	}

	fmt.Printf("Creating Kind cluster %q with config %q...\n", c.Name, configPath)
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

	fmt.Printf("Deleting Kind cluster %q...\n", c.Name)
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

	fmt.Printf("Loading Docker image %q into Kind cluster %q...\n", image, c.Name)
	return RunCommand(ctx, "kind", "load", "docker-image", image, "--name", c.Name)
}

// ExportKubeconfig exports the cluster kubeconfig to a temporary file and returns the path.
func (c *Cluster) ExportKubeconfig(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("cluster is nil")
	}

	tempFile, err := os.CreateTemp("", fmt.Sprintf("kubeconfig-%s-*.yaml", c.Name))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for kubeconfig: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	fmt.Printf("Exporting kubeconfig for cluster %q to %q...\n", c.Name, tempPath)
	output, err := RunCommandOutput(ctx, "kind", "get", "kubeconfig", "--name", c.Name)
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig from kind: %w", err)
	}

	if err := os.WriteFile(tempPath, []byte(output), 0o600); err != nil {
		return "", fmt.Errorf("failed to write kubeconfig to %q: %w", tempPath, err)
	}

	c.Kubeconfig = tempPath
	return tempPath, nil
}
