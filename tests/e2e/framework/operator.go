package framework

import (
	"context"
	"fmt"
)

// InstallOperator installs or upgrades the infra-operator chart.
func InstallOperator(ctx context.Context, cfg Config) error {
	if cfg.OperatorChartPath == "" {
		return fmt.Errorf("operator chart path is required")
	}

	setValues := []string{}
	if cfg.OperatorImageRepo != "" {
		setValues = append(setValues, fmt.Sprintf("image.repository=%s", cfg.OperatorImageRepo))
	}
	if cfg.OperatorImageTag != "" {
		setValues = append(setValues, fmt.Sprintf("image.tag=%s", cfg.OperatorImageTag))
	}

	return HelmUpgradeInstall(
		ctx,
		cfg.OperatorReleaseName,
		cfg.OperatorChartPath,
		cfg.OperatorNamespace,
		cfg.Kubeconfig,
		cfg.OperatorValuesPath,
		setValues,
	)
}

// UninstallOperator removes the infra-operator release.
func UninstallOperator(ctx context.Context, cfg Config) error {
	return HelmUninstall(ctx, cfg.OperatorReleaseName, cfg.OperatorNamespace, cfg.Kubeconfig)
}

// WaitForOperatorReady waits until the operator deployment is ready.
func WaitForOperatorReady(ctx context.Context, cfg Config, timeout string) error {
	return WaitForDeployment(ctx, cfg.Kubeconfig, cfg.OperatorNamespace, cfg.OperatorDeploymentName, timeout)
}
