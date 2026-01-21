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
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sandbox0-ai/infra-operator/api/v1alpha1"
)

const (
	finalizerName   = "sandbox0infra.infra.sandbox0.ai/finalizer"
	requeueInterval = 30 * time.Second
)

// Sandbox0InfraReconciler reconciles a Sandbox0Infra object
type Sandbox0InfraReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infra.sandbox0.ai,resources=sandbox0infras,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.sandbox0.ai,resources=sandbox0infras/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.sandbox0.ai,resources=sandbox0infras/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services;secrets;configmaps;persistentvolumeclaims;serviceaccounts;pods;pods/exec;nodes;events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sandbox0.ai,resources=sandboxtemplates,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Sandbox0InfraReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Sandbox0Infra", "namespace", req.Namespace, "name", req.Name)

	// Fetch the Sandbox0Infra instance
	infra := &infrav1alpha1.Sandbox0Infra{}
	if err := r.Get(ctx, req.NamespacedName, infra); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Sandbox0Infra resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Sandbox0Infra")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !infra.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, infra)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(infra, finalizerName) {
		controllerutil.AddFinalizer(infra, finalizerName)
		if err := r.Update(ctx, infra); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize status if needed
	if infra.Status.Phase == "" {
		return r.initializeStatus(ctx, infra)
	}

	// Set default values
	r.setDefaults(infra)

	// Main reconciliation logic based on mode
	var result ctrl.Result
	var err error

	switch infra.Spec.Mode {
	case infrav1alpha1.DeploymentModeAll:
		result, err = r.reconcileAllMode(ctx, infra)
	case infrav1alpha1.DeploymentModeControlPlane:
		result, err = r.reconcileControlPlaneMode(ctx, infra)
	case infrav1alpha1.DeploymentModeDataPlane:
		result, err = r.reconcileDataPlaneMode(ctx, infra)
	default:
		// Default to all mode
		result, err = r.reconcileAllMode(ctx, infra)
	}

	// Update overall status
	if updateErr := r.updateOverallStatus(ctx, infra); updateErr != nil {
		logger.Error(updateErr, "Failed to update overall status")
		if err == nil {
			err = updateErr
		}
	}

	return result, err
}

// setDefaults sets default values for the spec
func (r *Sandbox0InfraReconciler) setDefaults(infra *infrav1alpha1.Sandbox0Infra) {
	if infra.Spec.Mode == "" {
		infra.Spec.Mode = infrav1alpha1.DeploymentModeAll
	}
	if infra.Spec.Version == "" {
		infra.Spec.Version = "v0.1.0"
	}
	if infra.Spec.Database.Type == "" {
		infra.Spec.Database.Type = infrav1alpha1.DatabaseTypeBuiltin
	}
	if infra.Spec.Storage.Type == "" {
		infra.Spec.Storage.Type = infrav1alpha1.StorageTypeBuiltin
	}
}

// initializeStatus initializes the status for a new resource
func (r *Sandbox0InfraReconciler) initializeStatus(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Initializing status")

	now := metav1.Now()
	infra.Status.Phase = infrav1alpha1.PhaseInstalling
	infra.Status.ObservedVersion = infra.Spec.Version
	infra.Status.LastOperation = &infrav1alpha1.LastOperation{
		Type:      "Install",
		StartedAt: &now,
		Status:    "InProgress",
	}

	if err := r.Status().Update(ctx, infra); err != nil {
		logger.Error(err, "Failed to initialize status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

// reconcileDelete handles cleanup when the resource is being deleted
func (r *Sandbox0InfraReconciler) reconcileDelete(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion")

	// Cleanup logic here - resources owned by this CR will be garbage collected
	// but we might need to clean up external resources

	// Remove finalizer
	controllerutil.RemoveFinalizer(infra, finalizerName)
	if err := r.Update(ctx, infra); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileAllMode reconciles all components (local development mode)
func (r *Sandbox0InfraReconciler) reconcileAllMode(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling all mode")

	steps := []reconcileStep{
		{
			Name:           "internal-auth",
			Run:            func(ctx context.Context) error { return r.reconcileInternalAuth(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeInternalAuthReady,
			SuccessReason:  "KeysReady",
			SuccessMessage: "Internal auth keys are ready",
			ErrorReason:    "KeyGenerationFailed",
		},
		{
			Name:           "database",
			Run:            func(ctx context.Context) error { return r.reconcileDatabase(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeDatabaseReady,
			SuccessReason:  "DatabaseReady",
			SuccessMessage: "Database is ready",
			ErrorReason:    "DatabaseFailed",
		},
		{
			Name:           "storage",
			Run:            func(ctx context.Context) error { return r.reconcileStorage(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeStorageReady,
			SuccessReason:  "StorageReady",
			SuccessMessage: "Storage is ready",
			ErrorReason:    "StorageFailed",
		},
		{
			Name:           "edge-gateway",
			Run:            func(ctx context.Context) error { return r.reconcileEdgeGateway(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeEdgeGatewayReady,
			SuccessReason:  "EdgeGatewayReady",
			SuccessMessage: "Edge gateway is ready",
			ErrorReason:    "EdgeGatewayFailed",
		},
		{
			Name:                 "scheduler-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileSchedulerRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeSchedulerReady,
			ErrorReason:          "SchedulerRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "scheduler",
			Run:            func(ctx context.Context) error { return r.reconcileScheduler(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeSchedulerReady,
			SuccessReason:  "SchedulerReady",
			SuccessMessage: "Scheduler is ready",
			ErrorReason:    "SchedulerFailed",
		},
		{
			Name:           "internal-gateway",
			Run:            func(ctx context.Context) error { return r.reconcileInternalGateway(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeInternalGatewayReady,
			SuccessReason:  "InternalGatewayReady",
			SuccessMessage: "Internal gateway is ready",
			ErrorReason:    "InternalGatewayFailed",
		},
		{
			Name:                 "manager-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileManagerRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeManagerReady,
			ErrorReason:          "ManagerRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "manager",
			Run:            func(ctx context.Context) error { return r.reconcileManager(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeManagerReady,
			SuccessReason:  "ManagerReady",
			SuccessMessage: "Manager is ready",
			ErrorReason:    "ManagerFailed",
		},
		{
			Name:                 "storage-proxy-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileStorageProxyRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeStorageProxyReady,
			ErrorReason:          "StorageProxyRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "storage-proxy",
			Run:            func(ctx context.Context) error { return r.reconcileStorageProxy(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeStorageProxyReady,
			SuccessReason:  "StorageProxyReady",
			SuccessMessage: "Storage proxy is ready",
			ErrorReason:    "StorageProxyFailed",
		},
		{
			Name:                 "netd-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileNetdRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeNetdReady,
			ErrorReason:          "NetdRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "netd",
			Run:            func(ctx context.Context) error { return r.reconcileNetd(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeNetdReady,
			SuccessReason:  "NetdReady",
			SuccessMessage: "Netd is ready",
			ErrorReason:    "NetdFailed",
		},
		{
			Name: "init-user",
			Run: func(ctx context.Context) error {
				if infra.Spec.InitUser != nil && infra.Spec.InitUser.Enabled {
					return r.reconcileInitUser(ctx, infra)
				}
				return nil
			},
			IgnoreError: true,
		},
	}

	return r.runSteps(ctx, infra, steps)
}

// reconcileControlPlaneMode reconciles control plane components only
func (r *Sandbox0InfraReconciler) reconcileControlPlaneMode(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling control-plane mode")

	steps := []reconcileStep{
		{
			Name:           "internal-auth",
			Run:            func(ctx context.Context) error { return r.reconcileInternalAuth(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeInternalAuthReady,
			SuccessReason:  "KeysReady",
			SuccessMessage: "Internal auth keys are ready",
			ErrorReason:    "KeyGenerationFailed",
		},
		{
			Name:           "external-database",
			Run:            func(ctx context.Context) error { return r.validateExternalDatabase(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeDatabaseReady,
			SuccessReason:  "DatabaseReady",
			SuccessMessage: "External database connected",
			ErrorReason:    "DatabaseConnectionFailed",
		},
		{
			Name:           "external-storage",
			Run:            func(ctx context.Context) error { return r.validateExternalStorage(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeStorageReady,
			SuccessReason:  "StorageReady",
			SuccessMessage: "External storage accessible",
			ErrorReason:    "StorageConnectionFailed",
		},
		{
			Name:           "edge-gateway",
			Run:            func(ctx context.Context) error { return r.reconcileEdgeGateway(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeEdgeGatewayReady,
			SuccessReason:  "EdgeGatewayReady",
			SuccessMessage: "Edge gateway is ready",
			ErrorReason:    "EdgeGatewayFailed",
		},
		{
			Name:                 "scheduler-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileSchedulerRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeSchedulerReady,
			ErrorReason:          "SchedulerRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "scheduler",
			Run:            func(ctx context.Context) error { return r.reconcileScheduler(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeSchedulerReady,
			SuccessReason:  "SchedulerReady",
			SuccessMessage: "Scheduler is ready",
			ErrorReason:    "SchedulerFailed",
		},
	}

	return r.runSteps(ctx, infra, steps)
}

// reconcileDataPlaneMode reconciles data plane components only
func (r *Sandbox0InfraReconciler) reconcileDataPlaneMode(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling data-plane mode")

	steps := []reconcileStep{
		{
			Name: "control-plane-config",
			Run: func(ctx context.Context) error {
				if infra.Spec.ControlPlane == nil {
					return fmt.Errorf("controlPlane configuration is required for data-plane mode")
				}
				return nil
			},
			ConditionType:        infrav1alpha1.ConditionTypeInternalAuthReady,
			ErrorReason:          "MissingControlPlane",
			SkipSuccessCondition: true,
			ErrorResult:          &ctrl.Result{},
		},
		{
			Name: "control-plane-public-key",
			Run: func(ctx context.Context) error {
				publicKeySecret := &corev1.Secret{}
				return r.Get(ctx, types.NamespacedName{
					Name:      infra.Spec.ControlPlane.InternalAuthPublicKeySecret.Name,
					Namespace: infra.Namespace,
				}, publicKeySecret)
			},
			ConditionType:        infrav1alpha1.ConditionTypeInternalAuthReady,
			ErrorReason:          "PublicKeySecretNotFound",
			SkipSuccessCondition: true,
		},
		{
			Name:           "internal-auth",
			Run:            func(ctx context.Context) error { return r.reconcileInternalAuth(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeInternalAuthReady,
			SuccessReason:  "KeysReady",
			SuccessMessage: "Internal auth keys are ready",
			ErrorReason:    "KeyGenerationFailed",
		},
		{
			Name:           "external-database",
			Run:            func(ctx context.Context) error { return r.validateExternalDatabase(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeDatabaseReady,
			SuccessReason:  "DatabaseReady",
			SuccessMessage: "External database connected",
			ErrorReason:    "DatabaseConnectionFailed",
		},
		{
			Name:           "external-storage",
			Run:            func(ctx context.Context) error { return r.validateExternalStorage(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeStorageReady,
			SuccessReason:  "StorageReady",
			SuccessMessage: "External storage accessible",
			ErrorReason:    "StorageConnectionFailed",
		},
		{
			Name:           "internal-gateway",
			Run:            func(ctx context.Context) error { return r.reconcileInternalGateway(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeInternalGatewayReady,
			SuccessReason:  "InternalGatewayReady",
			SuccessMessage: "Internal gateway is ready",
			ErrorReason:    "InternalGatewayFailed",
		},
		{
			Name:                 "manager-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileManagerRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeManagerReady,
			ErrorReason:          "ManagerRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "manager",
			Run:            func(ctx context.Context) error { return r.reconcileManager(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeManagerReady,
			SuccessReason:  "ManagerReady",
			SuccessMessage: "Manager is ready",
			ErrorReason:    "ManagerFailed",
		},
		{
			Name:                 "storage-proxy-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileStorageProxyRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeStorageProxyReady,
			ErrorReason:          "StorageProxyRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "storage-proxy",
			Run:            func(ctx context.Context) error { return r.reconcileStorageProxy(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeStorageProxyReady,
			SuccessReason:  "StorageProxyReady",
			SuccessMessage: "Storage proxy is ready",
			ErrorReason:    "StorageProxyFailed",
		},
		{
			Name:                 "netd-rbac",
			Run:                  func(ctx context.Context) error { return r.reconcileNetdRBAC(ctx, infra) },
			ConditionType:        infrav1alpha1.ConditionTypeNetdReady,
			ErrorReason:          "NetdRBACFailed",
			SkipSuccessCondition: true,
		},
		{
			Name:           "netd",
			Run:            func(ctx context.Context) error { return r.reconcileNetd(ctx, infra) },
			ConditionType:  infrav1alpha1.ConditionTypeNetdReady,
			SuccessReason:  "NetdReady",
			SuccessMessage: "Netd is ready",
			ErrorReason:    "NetdFailed",
		},
		{
			Name: "register-cluster",
			Run: func(ctx context.Context) error {
				if infra.Spec.Cluster != nil {
					return r.registerCluster(ctx, infra)
				}
				return nil
			},
			IgnoreError: true,
		},
	}

	return r.runSteps(ctx, infra, steps)
}

// updateOverallStatus updates the overall status based on conditions
func (r *Sandbox0InfraReconciler) updateOverallStatus(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)
	original := infra.Status.DeepCopy()

	// Check if all required conditions are true
	allReady := true
	for _, cond := range infra.Status.Conditions {
		if cond.Status != metav1.ConditionTrue {
			allReady = false
			break
		}
	}

	// Update phase
	if allReady && len(infra.Status.Conditions) > 0 {
		infra.Status.Phase = infrav1alpha1.PhaseReady
		if infra.Status.LastOperation != nil && infra.Status.LastOperation.Status == "InProgress" {
			now := metav1.Now()
			infra.Status.LastOperation.CompletedAt = &now
			infra.Status.LastOperation.Status = "Succeeded"
		}
	} else {
		// Check if any condition failed
		hasFailed := false
		for _, cond := range infra.Status.Conditions {
			if cond.Status == metav1.ConditionFalse && cond.Reason != "" {
				hasFailed = true
				break
			}
		}
		if hasFailed {
			infra.Status.Phase = infrav1alpha1.PhaseDegraded
		}
	}

	// Set the Ready condition
	if allReady {
		r.setCondition(ctx, infra, infrav1alpha1.ConditionTypeReady, metav1.ConditionTrue, "AllServicesHealthy", "All services are healthy")
	} else {
		r.setCondition(ctx, infra, infrav1alpha1.ConditionTypeReady, metav1.ConditionFalse, "ServicesNotReady", "Some services are not ready")
	}

	// Update status
	if reflect.DeepEqual(original, &infra.Status) {
		return nil
	}
	if err := r.Status().Update(ctx, infra); err != nil {
		logger.Error(err, "Failed to update status")
		return err
	}

	return nil
}

// setCondition sets or updates a condition
func (r *Sandbox0InfraReconciler) setCondition(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&infra.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: infra.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// validateExternalDatabase validates connection to external database
func (r *Sandbox0InfraReconciler) validateExternalDatabase(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	if infra.Spec.Database.External == nil {
		return fmt.Errorf("external database configuration is required")
	}

	// Check if password secret exists
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      infra.Spec.Database.External.PasswordSecret.Name,
		Namespace: infra.Namespace,
	}, secret); err != nil {
		return fmt.Errorf("database password secret not found: %w", err)
	}

	key := infra.Spec.Database.External.PasswordSecret.Key
	if key == "" {
		key = "password"
	}
	if _, ok := secret.Data[key]; !ok {
		return fmt.Errorf("key %s not found in database password secret", key)
	}

	return nil
}

// validateExternalStorage validates external storage configuration
func (r *Sandbox0InfraReconciler) validateExternalStorage(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	switch infra.Spec.Storage.Type {
	case infrav1alpha1.StorageTypeS3:
		if infra.Spec.Storage.S3 == nil {
			return fmt.Errorf("S3 configuration is required")
		}
		// Check credentials secret
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      infra.Spec.Storage.S3.CredentialsSecret.Name,
			Namespace: infra.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("S3 credentials secret not found: %w", err)
		}

	case infrav1alpha1.StorageTypeOSS:
		if infra.Spec.Storage.OSS == nil {
			return fmt.Errorf("OSS configuration is required")
		}
		// Check credentials secret
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      infra.Spec.Storage.OSS.CredentialsSecret.Name,
			Namespace: infra.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("OSS credentials secret not found: %w", err)
		}
	}

	return nil
}

// reconcileInitUser creates the initial admin user
func (r *Sandbox0InfraReconciler) reconcileInitUser(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	// This would typically call an API to create the user
	// For now, we just log that we would create the user
	logger := log.FromContext(ctx)
	logger.Info("Would create init user", "email", infra.Spec.InitUser.Email)
	return nil
}

// registerCluster registers the cluster with the control plane
func (r *Sandbox0InfraReconciler) registerCluster(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	if infra.Status.Cluster == nil {
		infra.Status.Cluster = &infrav1alpha1.ClusterStatus{}
	}

	if infra.Status.Cluster.Registered {
		return nil
	}

	// TODO: Implement actual registration with control plane
	logger.Info("Would register cluster with control plane",
		"clusterId", infra.Spec.Cluster.ID,
		"controlPlaneUrl", infra.Spec.ControlPlane.URL)

	now := metav1.Now()
	infra.Status.Cluster.ID = infra.Spec.Cluster.ID
	infra.Status.Cluster.Registered = true
	infra.Status.Cluster.RegisteredAt = &now

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Sandbox0InfraReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.Sandbox0Infra{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
