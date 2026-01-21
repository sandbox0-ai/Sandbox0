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

	infrav1alpha1 "github.com/sandbox0-ai/infra-operator/api/v1alpha1"
)

// reconcileEdgeGateway reconciles the edge-gateway deployment
func (r *Sandbox0InfraReconciler) reconcileEdgeGateway(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil && !infra.Spec.Services.EdgeGateway.Enabled {
		logger.Info("Edge gateway is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-edge-gateway", infra.Name)
	serviceName := deploymentName

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil {
		replicas = infra.Spec.Services.EdgeGateway.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "edge-gateway")
	keySecretName, privateKeyKey, _ := r.getControlPlaneKeyRefs(infra)

	config, err := r.buildEdgeGatewayConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:       "edge-gateway",
		Port:       8080,
		TargetPort: 8080,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
			},
		},
		Image: fmt.Sprintf("%s:%s", r.getImageRepo(ctx), infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "edge-gateway",
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
	servicePort := int32(80)
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil && infra.Spec.Services.EdgeGateway.Service != nil {
		serviceType = infra.Spec.Services.EdgeGateway.Service.Type
		servicePort = infra.Spec.Services.EdgeGateway.Service.Port
	}
	if err := r.reconcileService(ctx, infra, serviceName, labels, serviceType, servicePort, 8080); err != nil {
		return err
	}

	// Create ingress if enabled
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil &&
		infra.Spec.Services.EdgeGateway.Ingress != nil && infra.Spec.Services.EdgeGateway.Ingress.Enabled {
		if err := r.reconcileIngress(ctx, infra, serviceName, infra.Spec.Services.EdgeGateway.Ingress); err != nil {
			return err
		}
	}

	// Update endpoints in status
	r.updateEndpoints(ctx, infra, serviceName, servicePort)

	logger.Info("Edge gateway reconciled successfully")
	return nil
}

// reconcileScheduler reconciles the scheduler deployment
func (r *Sandbox0InfraReconciler) reconcileScheduler(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled (scheduler is optional by default)
	if infra.Spec.Services == nil || infra.Spec.Services.Scheduler == nil || !infra.Spec.Services.Scheduler.Enabled {
		logger.Info("Scheduler is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-scheduler", infra.Name)
	replicas := infra.Spec.Services.Scheduler.Replicas
	labels := r.getServiceLabels(infra.Name, "scheduler")
	keySecretName, privateKeyKey, publicKeyKey := r.getControlPlaneKeyRefs(infra)

	config, err := r.buildSchedulerConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:               "scheduler",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-scheduler", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
			},
		},
		Image: fmt.Sprintf("%s:%s", r.getImageRepo(ctx), infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "scheduler",
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
	if infra.Spec.Services != nil && infra.Spec.Services.Scheduler != nil && infra.Spec.Services.Scheduler.Service != nil {
		serviceType = infra.Spec.Services.Scheduler.Service.Type
		servicePort = infra.Spec.Services.Scheduler.Service.Port
	}
	if err := r.reconcileService(ctx, infra, deploymentName, labels, serviceType, servicePort, 8080); err != nil {
		return err
	}

	logger.Info("Scheduler reconciled successfully")
	return nil
}
