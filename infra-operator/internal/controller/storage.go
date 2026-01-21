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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sandbox0-ai/infra-operator/api/v1alpha1"
)

const (
	rustfsSecretName = "sandbox0-rustfs-credentials"
	rustfsPort       = 9000
	rustfsConsole    = 9001
)

// reconcileStorage reconciles the storage component
func (r *Sandbox0InfraReconciler) reconcileStorage(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	switch infra.Spec.Storage.Type {
	case infrav1alpha1.StorageTypeBuiltin:
		logger.Info("Reconciling builtin storage (RustFS)")
		return r.reconcileBuiltinStorage(ctx, infra)
	case infrav1alpha1.StorageTypeS3, infrav1alpha1.StorageTypeOSS:
		logger.Info("Using external storage")
		return r.validateExternalStorage(ctx, infra)
	default:
		return r.reconcileBuiltinStorage(ctx, infra)
	}
}

// reconcileBuiltinStorage creates a single-node RustFS (MinIO-compatible) deployment
func (r *Sandbox0InfraReconciler) reconcileBuiltinStorage(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Create storage credentials secret
	if err := r.reconcileStorageSecret(ctx, infra); err != nil {
		return err
	}

	// Create PVC for storage
	if err := r.reconcileStoragePVC(ctx, infra); err != nil {
		return err
	}

	// Create StatefulSet for RustFS/MinIO
	if err := r.reconcileStorageStatefulSet(ctx, infra); err != nil {
		return err
	}

	// Create Service for RustFS/MinIO
	if err := r.reconcileStorageService(ctx, infra); err != nil {
		return err
	}

	logger.Info("Builtin storage reconciled successfully")
	return nil
}

// reconcileStorageSecret creates or updates the storage credentials secret
func (r *Sandbox0InfraReconciler) reconcileStorageSecret(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	secretName := fmt.Sprintf("%s-%s", infra.Name, rustfsSecretName)

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: infra.Namespace}, secret)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		accessKey := "sandbox0admin"
		secretKey := generateRandomString(32)

		// Use configured credentials if provided
		if infra.Spec.Storage.Builtin != nil && infra.Spec.Storage.Builtin.Credentials != nil {
			if infra.Spec.Storage.Builtin.Credentials.AccessKey != "" {
				accessKey = infra.Spec.Storage.Builtin.Credentials.AccessKey
			}
			if infra.Spec.Storage.Builtin.Credentials.SecretKey != "" {
				secretKey = infra.Spec.Storage.Builtin.Credentials.SecretKey
			}
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: infra.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"RUSTFS_ACCESS_KEY": accessKey,
				"RUSTFS_SECRET_KEY": secretKey,
				"endpoint":          fmt.Sprintf("http://%s-rustfs:%d", infra.Name, rustfsPort),
			},
		}

		if err := ctrl.SetControllerReference(infra, secret, r.Scheme); err != nil {
			return err
		}

		return r.Create(ctx, secret)
	}

	return nil
}

// reconcileStoragePVC creates or updates the storage PVC
func (r *Sandbox0InfraReconciler) reconcileStoragePVC(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	pvcName := fmt.Sprintf("%s-rustfs-data", infra.Name)

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: infra.Namespace}, pvc)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		size := resource.MustParse("50Gi")
		if infra.Spec.Storage.Builtin != nil && infra.Spec.Storage.Builtin.Persistence != nil {
			size = infra.Spec.Storage.Builtin.Persistence.Size
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: infra.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: size,
					},
				},
			},
		}

		// Set storage class if specified
		if infra.Spec.Storage.Builtin != nil &&
			infra.Spec.Storage.Builtin.Persistence != nil &&
			infra.Spec.Storage.Builtin.Persistence.StorageClass != "" {
			pvc.Spec.StorageClassName = &infra.Spec.Storage.Builtin.Persistence.StorageClass
		}

		if err := ctrl.SetControllerReference(infra, pvc, r.Scheme); err != nil {
			return err
		}

		return r.Create(ctx, pvc)
	}

	return nil
}

// reconcileStorageStatefulSet creates or updates the RustFS/MinIO StatefulSet
func (r *Sandbox0InfraReconciler) reconcileStorageStatefulSet(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	stsName := fmt.Sprintf("%s-rustfs", infra.Name)
	secretName := fmt.Sprintf("%s-%s", infra.Name, rustfsSecretName)
	pvcName := fmt.Sprintf("%s-rustfs-data", infra.Name)

	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: infra.Namespace}, sts)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	replicas := int32(1)
	labels := map[string]string{
		"app.kubernetes.io/name":       "rustfs",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/component":  "storage",
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}

	// Using RustFS as the built-in S3-compatible storage
	desiredSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stsName,
			Namespace: infra.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: stsName,
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "rustfs",
							Image:   "rustfs/rustfs:1.0.0-alpha.79",
							Command: []string{"/usr/bin/rustfs"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "endpoint",
									ContainerPort: rustfsPort,
								},
								{
									Name:          "console",
									ContainerPort: rustfsConsole,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "RUSTFS_ACCESS_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "RUSTFS_ACCESS_KEY",
										},
									},
								},
								{
									Name: "RUSTFS_SECRET_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "RUSTFS_SECRET_KEY",
										},
									},
								},
								{
									Name:  "RUSTFS_ADDRESS",
									Value: fmt.Sprintf(":%d", rustfsPort),
								},
								{
									Name:  "RUSTFS_CONSOLE_ADDRESS",
									Value: fmt.Sprintf(":%d", rustfsConsole),
								},
								{
									Name:  "RUSTFS_CONSOLE_ENABLE",
									Value: "true",
								},
								{
									Name:  "RUSTFS_VOLUMES",
									Value: "/data",
								},
								{
									Name:  "RUSTFS_REGION",
									Value: "us-east-1",
								},
								{
									Name:  "RUSTFS_OBS_LOG_DIRECTORY",
									Value: "/data/logs",
								},
								{
									Name:  "RUSTFS_OBS_LOGGER_LEVEL",
									Value: "debug",
								},
								{
									Name:  "RUSTFS_OBS_ENVIRONMENT",
									Value: "develop",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(rustfsPort),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(rustfsPort),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(infra, desiredSts, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredSts)
	}

	// Update existing StatefulSet
	sts.Spec = desiredSts.Spec
	return r.Update(ctx, sts)
}

// reconcileStorageService creates or updates the RustFS/MinIO Service
func (r *Sandbox0InfraReconciler) reconcileStorageService(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	svcName := fmt.Sprintf("%s-rustfs", infra.Name)

	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: infra.Namespace}, svc)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "rustfs",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/component":  "storage",
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}

	desiredSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: infra.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "api",
					Port:       rustfsPort,
					TargetPort: intstr.FromInt(rustfsPort),
				},
				{
					Name:       "console",
					Port:       rustfsConsole,
					TargetPort: intstr.FromInt(rustfsConsole),
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

	// Update existing Service
	svc.Spec = desiredSvc.Spec
	return r.Update(ctx, svc)
}

// StorageConfig contains storage configuration for services
type StorageConfig struct {
	Type         infrav1alpha1.StorageType
	Endpoint     string
	Bucket       string
	Region       string
	AccessKey    string
	SecretKey    string
	SessionToken string
	SecretName   string
}

// getStorageConfig returns the storage configuration for services to use
func (r *Sandbox0InfraReconciler) getStorageConfig(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (*StorageConfig, error) {
	config := &StorageConfig{
		Type: infra.Spec.Storage.Type,
	}

	switch infra.Spec.Storage.Type {
	case infrav1alpha1.StorageTypeBuiltin:
		secretName := fmt.Sprintf("%s-%s", infra.Name, rustfsSecretName)
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: infra.Namespace}, secret); err != nil {
			return nil, err
		}
		config.Endpoint = string(secret.Data["endpoint"])
		config.AccessKey = string(secret.Data["RUSTFS_ACCESS_KEY"])
		config.SecretKey = string(secret.Data["RUSTFS_SECRET_KEY"])
		config.SecretName = secretName
		config.Bucket = "sandbox0"
		config.Region = "us-east-1"

	case infrav1alpha1.StorageTypeS3:
		if infra.Spec.Storage.S3 == nil {
			return nil, fmt.Errorf("S3 configuration is required")
		}
		s3 := infra.Spec.Storage.S3
		config.Bucket = s3.Bucket
		config.Region = s3.Region
		config.Endpoint = s3.Endpoint
		config.SecretName = s3.CredentialsSecret.Name

		// Get credentials from secret
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: s3.CredentialsSecret.Name, Namespace: infra.Namespace}, secret); err != nil {
			return nil, err
		}
		accessKeyKey := s3.CredentialsSecret.AccessKeyKey
		if accessKeyKey == "" {
			accessKeyKey = "accessKeyId"
		}
		secretKeyKey := s3.CredentialsSecret.SecretKeyKey
		if secretKeyKey == "" {
			secretKeyKey = "secretAccessKey"
		}
		config.AccessKey = string(secret.Data[accessKeyKey])
		config.SecretKey = string(secret.Data[secretKeyKey])
		sessionTokenKey := s3.SessionTokenKey
		if sessionTokenKey != "" {
			config.SessionToken = string(secret.Data[sessionTokenKey])
		}

	case infrav1alpha1.StorageTypeOSS:
		if infra.Spec.Storage.OSS == nil {
			return nil, fmt.Errorf("OSS configuration is required")
		}
		oss := infra.Spec.Storage.OSS
		config.Bucket = oss.Bucket
		config.Region = oss.Region
		config.Endpoint = oss.Endpoint
		config.SecretName = oss.CredentialsSecret.Name

		// Get credentials from secret
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: oss.CredentialsSecret.Name, Namespace: infra.Namespace}, secret); err != nil {
			return nil, err
		}
		accessKeyKey := oss.CredentialsSecret.AccessKeyKey
		if accessKeyKey == "" {
			accessKeyKey = "accessKeyId"
		}
		secretKeyKey := oss.CredentialsSecret.SecretKeyKey
		if secretKeyKey == "" {
			secretKeyKey = "accessKeySecret"
		}
		config.AccessKey = string(secret.Data[accessKeyKey])
		config.SecretKey = string(secret.Data[secretKeyKey])

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", infra.Spec.Storage.Type)
	}

	return config, nil
}
