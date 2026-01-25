package framework

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// GetServicePort returns the first port exposed by a service.
func GetServicePort(ctx context.Context, kubeconfig, namespace, service string) (int, error) {
	if service == "" {
		return 0, fmt.Errorf("service name is required")
	}

	args := []string{"get", "svc", service, "-o", "jsonpath={.spec.ports[0].port}"}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	output, err := KubectlOutput(ctx, kubeconfig, args...)
	if err != nil {
		return 0, err
	}

	value := strings.TrimSpace(output)
	if value == "" {
		return 0, fmt.Errorf("service %q has no ports", service)
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse service port %q: %w", value, err)
	}
	return port, nil
}

// GetSecretValue reads a base64-encoded secret key value.
func GetSecretValue(ctx context.Context, kubeconfig, namespace, secretName, key string) (string, error) {
	if secretName == "" || key == "" {
		return "", fmt.Errorf("secret name and key are required")
	}

	jsonPath := fmt.Sprintf("{.data.%s}", key)
	args := []string{"get", "secret", secretName, "-o", fmt.Sprintf("jsonpath=%s", jsonPath)}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	output, err := KubectlOutput(ctx, kubeconfig, args...)
	if err != nil {
		return "", err
	}
	encoded := strings.TrimSpace(output)
	if encoded == "" {
		return "", fmt.Errorf("secret %q key %q is empty", secretName, key)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode secret %q key %q: %w", secretName, key, err)
	}
	return string(decoded), nil
}
