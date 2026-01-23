package framework

import (
	"context"
	"fmt"
	"strings"
)

// Kubectl runs a kubectl command with optional kubeconfig.
func Kubectl(ctx context.Context, kubeconfig string, args ...string) error {
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	return RunCommand(ctx, "kubectl", args...)
}

// KubectlOutput runs a kubectl command and returns output.
func KubectlOutput(ctx context.Context, kubeconfig string, args ...string) (string, error) {
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	return RunCommandOutput(ctx, "kubectl", args...)
}

// KubectlWaitForCondition waits for a condition on a resource.
func KubectlWaitForCondition(ctx context.Context, kubeconfig, namespace, resource, name, condition, timeout string) error {
	if resource == "" || name == "" || condition == "" {
		return fmt.Errorf("resource, name, and condition are required")
	}

	args := []string{
		"wait",
		fmt.Sprintf("--for=condition=%s", condition),
		fmt.Sprintf("%s/%s", resource, name),
		"--timeout",
		timeout,
	}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	return Kubectl(ctx, kubeconfig, args...)
}

// KubectlWaitForDelete waits for a resource to be deleted.
func KubectlWaitForDelete(ctx context.Context, kubeconfig, namespace, resource, name, timeout string) error {
	if resource == "" || name == "" {
		return fmt.Errorf("resource and name are required")
	}

	args := []string{
		"wait",
		"--for=delete",
		fmt.Sprintf("%s/%s", resource, name),
		"--timeout",
		timeout,
	}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	return Kubectl(ctx, kubeconfig, args...)
}

// KubectlRolloutStatus waits for a rollout to finish.
func KubectlRolloutStatus(ctx context.Context, kubeconfig, namespace, resource, timeout string) error {
	if resource == "" {
		return fmt.Errorf("resource is required")
	}

	args := []string{
		"rollout",
		"status",
		resource,
		"--timeout",
		timeout,
	}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	return Kubectl(ctx, kubeconfig, args...)
}

// KubectlGetJSONPath fetches a jsonpath value for a resource.
func KubectlGetJSONPath(ctx context.Context, kubeconfig, namespace, resource, name, jsonPath string) (string, error) {
	if resource == "" || name == "" || jsonPath == "" {
		return "", fmt.Errorf("resource, name, and jsonPath are required")
	}

	args := []string{"get", resource, name, "-o", fmt.Sprintf("jsonpath=%s", jsonPath)}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	output, err := KubectlOutput(ctx, kubeconfig, args...)
	return strings.TrimSpace(output), err
}

// KubectlPatch applies a merge patch to a resource.
func KubectlPatch(ctx context.Context, kubeconfig, namespace, resource, name, patch string) error {
	if resource == "" || name == "" || patch == "" {
		return fmt.Errorf("resource, name, and patch are required")
	}

	args := []string{
		"patch",
		resource,
		name,
		"--type",
		"merge",
		"-p",
		patch,
	}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	return Kubectl(ctx, kubeconfig, args...)
}

// KubectlDeleteManifest deletes resources from a manifest file.
func KubectlDeleteManifest(ctx context.Context, kubeconfig, manifestPath string) error {
	if manifestPath == "" {
		return fmt.Errorf("manifest path is required")
	}
	return Kubectl(ctx, kubeconfig, "delete", "-f", manifestPath, "--ignore-not-found=true")
}
