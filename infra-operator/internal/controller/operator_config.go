/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorconfig "github.com/sandbox0-ai/infra/infra-operator/api/config"
)

func (r *Sandbox0InfraReconciler) getImageRepo(ctx context.Context) string {
	logger := log.FromContext(ctx)

	config, err := operatorconfig.LoadOperatorConfig(operatorconfig.DefaultConfigPath)
	if err != nil {
		logger.Error(err, "Failed to load operator config")
		return operatorconfig.DefaultImageRepo
	}

	return config.ImageRepo
}

func (r *Sandbox0InfraReconciler) getImagePullPolicy(ctx context.Context) *corev1.PullPolicy {
	logger := log.FromContext(ctx)

	config, err := operatorconfig.LoadOperatorConfig(operatorconfig.DefaultConfigPath)
	if err != nil {
		logger.Error(err, "Failed to load operator config")
		return nil
	}

	policy := parsePullPolicy(config.ImagePullPolicy)
	if policy == nil && strings.TrimSpace(config.ImagePullPolicy) != "" {
		logger.Info("Ignoring invalid imagePullPolicy in operator config", "value", config.ImagePullPolicy)
	}
	return policy
}

func parsePullPolicy(raw string) *corev1.PullPolicy {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case strings.ToLower(string(corev1.PullAlways)):
		policy := corev1.PullAlways
		return &policy
	case strings.ToLower(string(corev1.PullIfNotPresent)):
		policy := corev1.PullIfNotPresent
		return &policy
	case strings.ToLower(string(corev1.PullNever)):
		policy := corev1.PullNever
		return &policy
	default:
		return nil
	}
}
