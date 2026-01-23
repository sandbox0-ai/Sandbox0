package framework

import (
	"context"
	"fmt"
)

// InfraConfig identifies a Sandbox0Infra instance for E2E tests.
type InfraConfig struct {
	Name      string
	Namespace string
}

// WaitForSandbox0InfraReady waits for the Ready condition.
func WaitForSandbox0InfraReady(ctx context.Context, kubeconfig string, infra InfraConfig, timeout string) error {
	if infra.Name == "" || infra.Namespace == "" {
		return fmt.Errorf("infra name and namespace are required")
	}
	return KubectlWaitForCondition(ctx, kubeconfig, infra.Namespace, "sandbox0infra", infra.Name, "Ready", timeout)
}
