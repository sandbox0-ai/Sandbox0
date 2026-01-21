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
	databaseName       = "postgres"
	databaseSecretName = "sandbox0-database-credentials"
	databasePort       = 5432
)

// reconcileDatabase reconciles the database component
func (r *Sandbox0InfraReconciler) reconcileDatabase(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	switch infra.Spec.Database.Type {
	case infrav1alpha1.DatabaseTypeBuiltin:
		logger.Info("Reconciling builtin database")
		return r.reconcileBuiltinDatabase(ctx, infra)
	case infrav1alpha1.DatabaseTypePostgres, infrav1alpha1.DatabaseTypeMySQL:
		logger.Info("Using external database")
		return r.validateExternalDatabase(ctx, infra)
	default:
		return r.reconcileBuiltinDatabase(ctx, infra)
	}
}

// reconcileBuiltinDatabase creates a single-node PostgreSQL StatefulSet
func (r *Sandbox0InfraReconciler) reconcileBuiltinDatabase(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Create database credentials secret
	if err := r.reconcileDatabaseSecret(ctx, infra); err != nil {
		return err
	}

	// Create PVC for database
	if err := r.reconcileDatabasePVC(ctx, infra); err != nil {
		return err
	}

	// Create StatefulSet for PostgreSQL
	if err := r.reconcileDatabaseStatefulSet(ctx, infra); err != nil {
		return err
	}

	// Create Service for PostgreSQL
	if err := r.reconcileDatabaseService(ctx, infra); err != nil {
		return err
	}

	logger.Info("Builtin database reconciled successfully")
	return nil
}

// reconcileDatabaseSecret creates or updates the database credentials secret
func (r *Sandbox0InfraReconciler) reconcileDatabaseSecret(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	secretName := fmt.Sprintf("%s-%s", infra.Name, databaseSecretName)

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: infra.Namespace}, secret)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		// Create new secret with generated password
		password := generateRandomString(24)
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: infra.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"username": "sandbox0",
				"password": password,
				"database": "sandbox0",
				"host":     fmt.Sprintf("%s-postgres", infra.Name),
				"port":     fmt.Sprintf("%d", databasePort),
				"dsn":      fmt.Sprintf("postgres://sandbox0:%s@%s-postgres:%d/sandbox0?sslmode=disable", password, infra.Name, databasePort),
			},
		}

		if err := ctrl.SetControllerReference(infra, secret, r.Scheme); err != nil {
			return err
		}

		return r.Create(ctx, secret)
	}

	return nil
}

// reconcileDatabasePVC creates or updates the database PVC
func (r *Sandbox0InfraReconciler) reconcileDatabasePVC(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	pvcName := fmt.Sprintf("%s-postgres-data", infra.Name)

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: infra.Namespace}, pvc)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		size := resource.MustParse("20Gi")
		if infra.Spec.Database.Builtin != nil && infra.Spec.Database.Builtin.Persistence != nil {
			size = infra.Spec.Database.Builtin.Persistence.Size
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
		if infra.Spec.Database.Builtin != nil &&
			infra.Spec.Database.Builtin.Persistence != nil &&
			infra.Spec.Database.Builtin.Persistence.StorageClass != "" {
			pvc.Spec.StorageClassName = &infra.Spec.Database.Builtin.Persistence.StorageClass
		}

		if err := ctrl.SetControllerReference(infra, pvc, r.Scheme); err != nil {
			return err
		}

		return r.Create(ctx, pvc)
	}

	return nil
}

// reconcileDatabaseStatefulSet creates or updates the PostgreSQL StatefulSet
func (r *Sandbox0InfraReconciler) reconcileDatabaseStatefulSet(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	stsName := fmt.Sprintf("%s-postgres", infra.Name)
	secretName := fmt.Sprintf("%s-%s", infra.Name, databaseSecretName)
	pvcName := fmt.Sprintf("%s-postgres-data", infra.Name)

	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: infra.Namespace}, sts)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	replicas := int32(1)
	labels := map[string]string{
		"app.kubernetes.io/name":       "postgres",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}

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
							Name:  "postgres",
							Image: "postgres:16-alpine",
							Ports: []corev1.ContainerPort{
								{
									Name:          "postgres",
									ContainerPort: databasePort,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "POSTGRES_USER",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "username",
										},
									},
								},
								{
									Name: "POSTGRES_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "password",
										},
									},
								},
								{
									Name: "POSTGRES_DB",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "database",
										},
									},
								},
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/postgresql/data",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "sandbox0"},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "sandbox0"},
									},
								},
								InitialDelaySeconds: 5,
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

// reconcileDatabaseService creates or updates the PostgreSQL Service
func (r *Sandbox0InfraReconciler) reconcileDatabaseService(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	svcName := fmt.Sprintf("%s-postgres", infra.Name)

	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: infra.Namespace}, svc)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "postgres",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/component":  "database",
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
					Name:       "postgres",
					Port:       databasePort,
					TargetPort: intstr.FromInt(databasePort),
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

// getDatabaseDSN returns the database DSN for services to use
func (r *Sandbox0InfraReconciler) getDatabaseDSN(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (string, error) {
	switch infra.Spec.Database.Type {
	case infrav1alpha1.DatabaseTypeBuiltin:
		secretName := fmt.Sprintf("%s-%s", infra.Name, databaseSecretName)
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: infra.Namespace}, secret); err != nil {
			return "", err
		}
		return string(secret.Data["dsn"]), nil

	case infrav1alpha1.DatabaseTypePostgres:
		if infra.Spec.Database.External == nil {
			return "", fmt.Errorf("external database configuration is required")
		}
		ext := infra.Spec.Database.External

		// Get password from secret
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: ext.PasswordSecret.Name, Namespace: infra.Namespace}, secret); err != nil {
			return "", err
		}

		key := ext.PasswordSecret.Key
		if key == "" {
			key = "password"
		}
		password := string(secret.Data[key])

		sslMode := ext.SSLMode
		if sslMode == "" {
			sslMode = "require"
		}

		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			ext.Username, password, ext.Host, ext.Port, ext.Database, sslMode), nil

	default:
		return "", fmt.Errorf("unsupported database type: %s", infra.Spec.Database.Type)
	}
}

func (r *Sandbox0InfraReconciler) getJuicefsMetaURL(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (string, error) {
	if infra.Spec.JuicefsDatabase == nil || infra.Spec.JuicefsDatabase.ShareWithMain {
		return r.getDatabaseDSN(ctx, infra)
	}

	ext := infra.Spec.JuicefsDatabase.External
	if ext == nil {
		return "", fmt.Errorf("juicefs external database configuration is required")
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ext.PasswordSecret.Name, Namespace: infra.Namespace}, secret); err != nil {
		return "", err
	}

	key := ext.PasswordSecret.Key
	if key == "" {
		key = "password"
	}
	password := string(secret.Data[key])

	sslMode := ext.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}

	port := ext.Port
	if port == 0 {
		port = 5432
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		ext.Username, password, ext.Host, port, ext.Database, sslMode), nil
}
