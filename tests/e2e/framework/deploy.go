package framework

import (
	"context"
	"fmt"
)

// ApplyManifest applies a YAML manifest to the cluster using kubectl.
func ApplyManifest(ctx context.Context, kubeconfig, manifestPath string) error {
	if manifestPath == "" {
		return fmt.Errorf("manifest path is required")
	}

	args := []string{"apply", "-f", manifestPath}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}

	return RunCommand(ctx, "kubectl", args...)
}

// WaitForDeployment waits until a deployment is ready.
func WaitForDeployment(ctx context.Context, kubeconfig, namespace, name string, timeout string) error {
	if name == "" {
		return fmt.Errorf("deployment name is required")
	}

	args := []string{
		"rollout",
		"status",
		"deployment",
		name,
		"--namespace",
		namespace,
		"--timeout",
		timeout,
	}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}

	return RunCommand(ctx, "kubectl", args...)
}
