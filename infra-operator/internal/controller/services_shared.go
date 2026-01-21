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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1alpha1 "github.com/sandbox0-ai/infra-operator/api/v1alpha1"
)

// ServiceDefinition defines deployment/daemonset configuration for a service.
type ServiceDefinition struct {
	Name               string
	Port               int32
	TargetPort         int32
	Ports              []corev1.ContainerPort
	Image              string
	Command            []string
	Args               []string
	EnvVars            []corev1.EnvVar
	VolumeMounts       []corev1.VolumeMount
	Volumes            []corev1.Volume
	LivenessProbe      *corev1.Probe
	ReadinessProbe     *corev1.Probe
	ServiceAccountName string
}

// reconcileDeployment creates or updates a deployment
func (r *Sandbox0InfraReconciler) reconcileDeployment(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, replicas int32, def ServiceDefinition) error {
	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, deploy)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	desiredDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: def.ServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:         def.Name,
							Image:        def.Image,
							Command:      def.Command,
							Args:         def.Args,
							Env:          def.EnvVars,
							VolumeMounts: def.VolumeMounts,
							Ports:        resolveContainerPorts(def),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							LivenessProbe:  def.LivenessProbe,
							ReadinessProbe: def.ReadinessProbe,
						},
					},
					Volumes: def.Volumes,
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(infra, desiredDeploy, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredDeploy)
	}

	deploy.Spec = desiredDeploy.Spec
	return r.Update(ctx, deploy)
}

// reconcileDaemonSet creates or updates a daemonset
func (r *Sandbox0InfraReconciler) reconcileDaemonSet(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, def ServiceDefinition) error {
	ds := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, ds)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	desiredDs := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: def.ServiceAccountName,
					HostNetwork:        true,
					HostPID:            true,
					DNSPolicy:          corev1.DNSClusterFirstWithHostNet,
					Containers: []corev1.Container{
						{
							Name:         def.Name,
							Image:        def.Image,
							Env:          def.EnvVars,
							VolumeMounts: def.VolumeMounts,
							Ports:        resolveContainerPorts(def),
							SecurityContext: &corev1.SecurityContext{
								Privileged: boolPtr(true),
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"NET_ADMIN", "SYS_ADMIN"},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							LivenessProbe:  def.LivenessProbe,
							ReadinessProbe: def.ReadinessProbe,
						},
					},
					Volumes: def.Volumes,
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(infra, desiredDs, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredDs)
	}

	ds.Spec = desiredDs.Spec
	return r.Update(ctx, ds)
}

func resolveContainerPorts(def ServiceDefinition) []corev1.ContainerPort {
	if len(def.Ports) > 0 {
		return def.Ports
	}
	if def.TargetPort == 0 {
		return nil
	}
	return []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: def.TargetPort,
		},
	}
}

// reconcileService creates or updates a service
func (r *Sandbox0InfraReconciler) reconcileService(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, serviceType corev1.ServiceType, port, targetPort int32) error {
	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, svc)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	desiredSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt(int(targetPort)),
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(infra, desiredSvc, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredSvc)
	}

	svc.Spec = desiredSvc.Spec
	return r.Update(ctx, svc)
}

// reconcileIngress creates or updates an ingress
func (r *Sandbox0InfraReconciler) reconcileIngress(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, serviceName string, config *infrav1alpha1.IngressConfig) error {
	ingressName := serviceName

	ingress := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: infra.Namespace}, ingress)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	pathType := networkingv1.PathTypePrefix
	desiredIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: infra.Namespace,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &config.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: config.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if config.TLSSecret != "" {
		desiredIngress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{config.Host},
				SecretName: config.TLSSecret,
			},
		}
	}

	if err := ctrl.SetControllerReference(infra, desiredIngress, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredIngress)
	}

	ingress.Spec = desiredIngress.Spec
	return r.Update(ctx, ingress)
}

// getServiceLabels returns standard labels for a service
func (r *Sandbox0InfraReconciler) getServiceLabels(instanceName, componentName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       componentName,
		"app.kubernetes.io/instance":   instanceName,
		"app.kubernetes.io/component":  componentName,
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}
}

// updateEndpoints updates the status endpoints
func (r *Sandbox0InfraReconciler) updateEndpoints(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, serviceName string, servicePort int32) {
	if infra.Status.Endpoints == nil {
		infra.Status.Endpoints = &infrav1alpha1.EndpointsStatus{}
	}

	internalURL := fmt.Sprintf("http://%s:%d", serviceName, servicePort)
	infra.Status.Endpoints.EdgeGatewayInternal = internalURL

	// If ingress is configured, set external URL
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil &&
		infra.Spec.Services.EdgeGateway.Ingress != nil && infra.Spec.Services.EdgeGateway.Ingress.Enabled {
		ingress := infra.Spec.Services.EdgeGateway.Ingress
		scheme := "http"
		if ingress.TLSSecret != "" {
			scheme = "https"
		}
		infra.Status.Endpoints.EdgeGateway = fmt.Sprintf("%s://%s", scheme, ingress.Host)
	}
}

// boolPtr returns a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}
