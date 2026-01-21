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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"

	infrav1alpha1 "github.com/sandbox0-ai/infra/infra-operator/api/v1alpha1"
)

func (r *Sandbox0InfraReconciler) reconcileServiceConfigMap(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, config map[string]any) error {
	if config == nil {
		config = map[string]any{}
	}

	payload, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config for %s: %w", name, err)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"config.yaml": string(payload),
		},
	}

	if err := ctrl.SetControllerReference(infra, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, existing)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}

	existing.Data = desired.Data
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *Sandbox0InfraReconciler) parseServiceConfig(raw *runtime.RawExtension) (map[string]any, error) {
	config := map[string]any{}
	if raw == nil || len(raw.Raw) == 0 {
		return config, nil
	}

	if err := yaml.Unmarshal(raw.Raw, &config); err != nil {
		return nil, fmt.Errorf("parse service config: %w", err)
	}

	return config, nil
}

func setIfMissing(config map[string]any, key string, value any) {
	if _, ok := config[key]; ok {
		return
	}
	config[key] = value
}

func getOrInitMap(config map[string]any, key string) map[string]any {
	if val, ok := config[key]; ok {
		if typed, ok := val.(map[string]any); ok {
			return typed
		}
	}

	child := map[string]any{}
	config[key] = child
	return child
}

func (r *Sandbox0InfraReconciler) buildEdgeGatewayConfig(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (map[string]any, error) {
	var raw *runtime.RawExtension
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil {
		raw = infra.Spec.Services.EdgeGateway.Config
	}

	config, err := r.parseServiceConfig(raw)
	if err != nil {
		return nil, err
	}

	if dsn, err := r.getDatabaseDSN(ctx, infra); err == nil {
		setIfMissing(config, "database_url", dsn)
	}

	internalGatewayURL := fmt.Sprintf("http://%s-internal-gateway:8443", infra.Name)
	setIfMissing(config, "default_internal_gateway_url", internalGatewayURL)

	if infra.Spec.InitUser != nil && infra.Spec.InitUser.Enabled {
		password, err := r.getSecretValue(ctx, infra.Namespace, infra.Spec.InitUser.PasswordSecret)
		if err != nil {
			return nil, err
		}

		builtInAuth := getOrInitMap(config, "built_in_auth")
		if _, ok := builtInAuth["init_user"]; !ok {
			builtInAuth["init_user"] = map[string]any{
				"email":    infra.Spec.InitUser.Email,
				"password": password,
				"name":     infra.Spec.InitUser.Name,
			}
		}
	}

	return config, nil
}

func (r *Sandbox0InfraReconciler) buildSchedulerConfig(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (map[string]any, error) {
	var raw *runtime.RawExtension
	if infra.Spec.Services != nil && infra.Spec.Services.Scheduler != nil {
		raw = infra.Spec.Services.Scheduler.Config
	}

	config, err := r.parseServiceConfig(raw)
	if err != nil {
		return nil, err
	}

	if dsn, err := r.getDatabaseDSN(ctx, infra); err == nil {
		setIfMissing(config, "database_url", dsn)
	}

	return config, nil
}

func (r *Sandbox0InfraReconciler) buildInternalGatewayConfig(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (map[string]any, error) {
	var raw *runtime.RawExtension
	if infra.Spec.Services != nil && infra.Spec.Services.InternalGateway != nil {
		raw = infra.Spec.Services.InternalGateway.Config
	}

	config, err := r.parseServiceConfig(raw)
	if err != nil {
		return nil, err
	}

	managerURL := fmt.Sprintf("http://%s-manager:8080", infra.Name)
	setIfMissing(config, "manager_url", managerURL)

	storageProxyURL := fmt.Sprintf("http://%s-storage-proxy-http:8081", infra.Name)
	setIfMissing(config, "storage_proxy_url", storageProxyURL)

	return config, nil
}

func (r *Sandbox0InfraReconciler) buildManagerConfig(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (map[string]any, error) {
	var raw *runtime.RawExtension
	if infra.Spec.Services != nil && infra.Spec.Services.Manager != nil {
		raw = infra.Spec.Services.Manager.Config
	}

	config, err := r.parseServiceConfig(raw)
	if err != nil {
		return nil, err
	}

	if dsn, err := r.getDatabaseDSN(ctx, infra); err == nil {
		setIfMissing(config, "database_url", dsn)
	}

	setIfMissing(config, "default_template_namespace", infra.Namespace)

	if infra.Spec.Cluster != nil && infra.Spec.Cluster.ID != "" {
		setIfMissing(config, "default_cluster_id", infra.Spec.Cluster.ID)
	}

	managerImage := fmt.Sprintf("%s:%s", r.getImageRepo(ctx), infra.Spec.Version)
	setIfMissing(config, "manager_image", managerImage)

	procdConfig := getOrInitMap(config, "procd_config")
	if _, ok := procdConfig["storage_proxy_base_url"]; !ok {
		procdConfig["storage_proxy_base_url"] = fmt.Sprintf("%s-storage-proxy.%s.svc.cluster.local", infra.Name, infra.Namespace)
	}

	return config, nil
}

func (r *Sandbox0InfraReconciler) buildStorageProxyConfig(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (map[string]any, error) {
	var raw *runtime.RawExtension
	if infra.Spec.Services != nil && infra.Spec.Services.StorageProxy != nil {
		raw = infra.Spec.Services.StorageProxy.Config
	}

	config, err := r.parseServiceConfig(raw)
	if err != nil {
		return nil, err
	}

	if dsn, err := r.getDatabaseDSN(ctx, infra); err == nil {
		setIfMissing(config, "database_url", dsn)
	}

	metaURL, err := r.getJuicefsMetaURL(ctx, infra)
	if err != nil {
		return nil, err
	}
	setIfMissing(config, "meta_url", metaURL)

	storageConfig, err := r.getStorageConfig(ctx, infra)
	if err != nil {
		return nil, err
	}

	setIfMissing(config, "s3_bucket", storageConfig.Bucket)
	setIfMissing(config, "s3_region", storageConfig.Region)
	setIfMissing(config, "s3_endpoint", storageConfig.Endpoint)
	setIfMissing(config, "s3_access_key", storageConfig.AccessKey)
	setIfMissing(config, "s3_secret_key", storageConfig.SecretKey)
	if storageConfig.SessionToken != "" {
		setIfMissing(config, "s3_session_token", storageConfig.SessionToken)
	}

	if infra.Spec.Cluster != nil && infra.Spec.Cluster.ID != "" {
		setIfMissing(config, "default_cluster_id", infra.Spec.Cluster.ID)
	}

	return config, nil
}

func (r *Sandbox0InfraReconciler) buildNetdConfig(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (map[string]any, error) {
	var raw *runtime.RawExtension
	if infra.Spec.Services != nil && infra.Spec.Services.Netd != nil {
		raw = infra.Spec.Services.Netd.Config
	}

	config, err := r.parseServiceConfig(raw)
	if err != nil {
		return nil, err
	}

	setIfMissing(config, "node_name", "${NODE_NAME}")
	setIfMissing(config, "namespace", infra.Namespace)

	return config, nil
}

func (r *Sandbox0InfraReconciler) getSecretValue(ctx context.Context, namespace string, ref infrav1alpha1.SecretKeyRef) (string, error) {
	if ref.Name == "" {
		return "", fmt.Errorf("secret name is required")
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", err
	}

	key := ref.Key
	if key == "" {
		key = "password"
	}

	value, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s", key, ref.Name)
	}

	return string(value), nil
}
