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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sandbox0-ai/infra/infra-operator/api/v1alpha1"
)

// reconcileInternalGateway reconciles the internal-gateway deployment
func (r *Sandbox0InfraReconciler) reconcileInternalGateway(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.InternalGateway != nil && !infra.Spec.Services.InternalGateway.Enabled {
		logger.Info("Internal gateway is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-internal-gateway", infra.Name)
	serviceName := deploymentName

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.InternalGateway != nil {
		replicas = infra.Spec.Services.InternalGateway.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "internal-gateway")
	dataPlaneSecretName, dataPlanePrivateKey, _ := r.getDataPlaneKeyRefs(infra)
	controlPlaneSecretName, _, controlPlanePublicKey := r.getControlPlaneKeyRefs(infra)

	config, err := r.buildInternalGatewayConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	controlPlanePublicSecretName, controlPlanePublicKeyKey := r.getControlPlanePublicKeyRef(infra)
	if controlPlanePublicSecretName == "" {
		controlPlanePublicSecretName = controlPlaneSecretName
		controlPlanePublicKeyKey = controlPlanePublicKey
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:       "internal-gateway",
		Port:       8443,
		TargetPort: 8443,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8443,
			},
		},
		Image: fmt.Sprintf("%s:%s", r.getImageRepo(ctx), infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "internal-gateway",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-private-key",
				MountPath: "/secrets/internal_jwt_private.key",
				SubPath:   "internal_jwt_private.key",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-public-key",
				MountPath: "/config/internal_jwt_public.key",
				SubPath:   "internal_jwt_public.key",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-private-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: dataPlaneSecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  dataPlanePrivateKey,
								Path: "internal_jwt_private.key",
							},
						},
					},
				},
			},
			{
				Name: "internal-jwt-public-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: controlPlanePublicSecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  controlPlanePublicKeyKey,
								Path: "internal_jwt_public.key",
							},
						},
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service
	if err := r.reconcileService(ctx, infra, serviceName, labels, corev1.ServiceTypeClusterIP, 8443, 8443); err != nil {
		return err
	}

	// Update endpoints in status
	if infra.Status.Endpoints == nil {
		infra.Status.Endpoints = &infrav1alpha1.EndpointsStatus{}
	}
	infra.Status.Endpoints.InternalGateway = fmt.Sprintf("http://%s:8443", serviceName)

	logger.Info("Internal gateway reconciled successfully")
	return nil
}

// reconcileManager reconciles the manager deployment
func (r *Sandbox0InfraReconciler) reconcileManager(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.Manager != nil && !infra.Spec.Services.Manager.Enabled {
		logger.Info("Manager is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-manager", infra.Name)

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.Manager != nil {
		replicas = infra.Spec.Services.Manager.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "manager")
	keySecretName, privateKeyKey, publicKeyKey := r.getDataPlaneKeyRefs(infra)

	config, err := r.buildManagerConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:               "manager",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-manager", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
			},
			{
				Name:          "metrics",
				ContainerPort: 9090,
			},
			{
				Name:          "webhook",
				ContainerPort: 9443,
			},
		},
		Image: fmt.Sprintf("%s:%s", r.getImageRepo(ctx), infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "manager",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-private-key",
				MountPath: "/secrets/internal_jwt_private.key",
				SubPath:   "internal_jwt_private.key",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-public-key",
				MountPath: "/config/internal_jwt_public.key",
				SubPath:   "internal_jwt_public.key",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-private-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  privateKeyKey,
								Path: "internal_jwt_private.key",
							},
						},
					},
				},
			},
			{
				Name: "internal-jwt-public-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  publicKeyKey,
								Path: "internal_jwt_public.key",
							},
						},
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service
	serviceType := corev1.ServiceTypeClusterIP
	servicePort := int32(8080)
	if infra.Spec.Services != nil && infra.Spec.Services.Manager != nil && infra.Spec.Services.Manager.Service != nil {
		serviceType = infra.Spec.Services.Manager.Service.Type
		servicePort = infra.Spec.Services.Manager.Service.Port
	}
	if err := r.reconcileService(ctx, infra, deploymentName, labels, serviceType, servicePort, 8080); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-metrics", deploymentName), labels, corev1.ServiceTypeClusterIP, 9090, 9090); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-webhook", deploymentName), labels, corev1.ServiceTypeClusterIP, 9443, 9443); err != nil {
		return err
	}

	logger.Info("Manager reconciled successfully")
	return nil
}

// reconcileStorageProxy reconciles the storage-proxy deployment
func (r *Sandbox0InfraReconciler) reconcileStorageProxy(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.StorageProxy != nil && !infra.Spec.Services.StorageProxy.Enabled {
		logger.Info("Storage proxy is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-storage-proxy", infra.Name)
	serviceName := deploymentName

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.StorageProxy != nil {
		replicas = infra.Spec.Services.StorageProxy.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "storage-proxy")
	keySecretName, _, publicKeyKey := r.getDataPlaneKeyRefs(infra)

	config, err := r.buildStorageProxyConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:               "storage-proxy",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-storage-proxy", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: 8080,
			},
			{
				Name:          "http",
				ContainerPort: 8081,
			},
			{
				Name:          "metrics",
				ContainerPort: 9090,
			},
		},
		Image: fmt.Sprintf("%s:%s", r.getImageRepo(ctx), infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "storage-proxy",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-public-key",
				MountPath: "/config/internal_jwt_public.key",
				SubPath:   "internal_jwt_public.key",
				ReadOnly:  true,
			},
			{
				Name:      "cache",
				MountPath: "/var/lib/storage-proxy/cache",
			},
			{
				Name:      "logs",
				MountPath: "/var/log/storage-proxy",
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-public-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  publicKeyKey,
								Path: "internal_jwt_public.key",
							},
						},
					},
				},
			},
			{
				Name: "cache",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "logs",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service (gRPC)
	if err := r.reconcileService(ctx, infra, serviceName, labels, corev1.ServiceTypeClusterIP, 8080, 8080); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-http", serviceName), labels, corev1.ServiceTypeClusterIP, 8081, 8081); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-metrics", serviceName), labels, corev1.ServiceTypeClusterIP, 9090, 9090); err != nil {
		return err
	}

	logger.Info("Storage proxy reconciled successfully")
	return nil
}

// reconcileNetd reconciles the netd daemonset
func (r *Sandbox0InfraReconciler) reconcileNetd(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.Netd != nil && !infra.Spec.Services.Netd.Enabled {
		logger.Info("Netd is disabled, skipping")
		return nil
	}

	dsName := fmt.Sprintf("%s-netd", infra.Name)
	labels := r.getServiceLabels(infra.Name, "netd")
	config, err := r.buildNetdConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, dsName, labels, config); err != nil {
		return err
	}

	// Create DaemonSet
	if err := r.reconcileDaemonSet(ctx, infra, dsName, labels, ServiceDefinition{
		Name:               "netd",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-netd", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 9090,
			},
			{
				Name:          "health",
				ContainerPort: 8080,
			},
			{
				Name:          "proxy-http",
				ContainerPort: 18080,
			},
			{
				Name:          "proxy-https",
				ContainerPort: 18443,
			},
		},
		Image: fmt.Sprintf("%s:%s", r.getImageRepo(ctx), infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "netd",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
			{
				Name: "NODE_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config",
				ReadOnly:  true,
			},
			{
				Name:             "bpf-fs",
				MountPath:        "/sys/fs/bpf",
				MountPropagation: func() *corev1.MountPropagationMode { mode := corev1.MountPropagationBidirectional; return &mode }(),
			},
			{
				Name:      "cgroup",
				MountPath: "/sys/fs/cgroup",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: dsName},
					},
				},
			},
			{
				Name: "bpf-fs",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/sys/fs/bpf",
						Type: func() *corev1.HostPathType { t := corev1.HostPathDirectoryOrCreate; return &t }(),
					},
				},
			},
			{
				Name: "cgroup",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/sys/fs/cgroup",
						Type: func() *corev1.HostPathType { t := corev1.HostPathDirectory; return &t }(),
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-metrics", dsName), labels, corev1.ServiceTypeClusterIP, 9090, 9090); err != nil {
		return err
	}

	logger.Info("Netd reconciled successfully")
	return nil
}
