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
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1alpha1 "github.com/sandbox0-ai/infra/infra-operator/api/v1alpha1"
)

// reconcileServiceAccount creates or updates a ServiceAccount for a service.
func (r *Sandbox0InfraReconciler) reconcileServiceAccount(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
			Labels:    labels,
		},
	}

	if err := ctrl.SetControllerReference(infra, sa, r.Scheme); err != nil {
		return err
	}

	found := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, sa)
	} else if err != nil {
		return err
	}

	return nil
}

// reconcileClusterRole creates or updates a ClusterRole for a service.
func (r *Sandbox0InfraReconciler) reconcileClusterRole(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, rules []rbacv1.PolicyRule) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Rules: rules,
	}

	// Cluster-scoped resources can't have owner references to namespaced resources.
	// We'll manage deletion manually or just leave them.

	found := &rbacv1.ClusterRole{}
	err := r.Get(ctx, types.NamespacedName{Name: name}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, role)
	} else if err != nil {
		return err
	}

	found.Rules = rules
	return r.Update(ctx, found)
}

// reconcileClusterRoleBinding creates or updates a ClusterRoleBinding for a service.
func (r *Sandbox0InfraReconciler) reconcileClusterRoleBinding(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, roleName string, saName string) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: infra.Namespace,
			},
		},
	}

	found := &rbacv1.ClusterRoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: name}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, binding)
	} else if err != nil {
		return err
	}

	found.RoleRef = binding.RoleRef
	found.Subjects = binding.Subjects
	return r.Update(ctx, found)
}

// reconcileManagerRBAC reconciles RBAC for the manager service.
func (r *Sandbox0InfraReconciler) reconcileManagerRBAC(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	name := fmt.Sprintf("%s-manager", infra.Name)
	labels := map[string]string{
		"app.kubernetes.io/name":       "manager",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}

	if err := r.reconcileServiceAccount(ctx, infra, name, labels); err != nil {
		return err
	}

	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{"sandbox0.ai"},
			Resources: []string{"sandboxtemplates", "sandboxtemplates/status"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "pods/exec", "pods/status", "services", "configmaps", "secrets", "persistentvolumeclaims", "events", "nodes"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"replicasets", "deployments"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"admissionregistration.k8s.io"},
			Resources: []string{"validatingwebhookconfigurations", "mutatingwebhookconfigurations"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
	}

	if err := r.reconcileClusterRole(ctx, infra, name, labels, rules); err != nil {
		return err
	}

	return r.reconcileClusterRoleBinding(ctx, infra, name, labels, name, name)
}

// reconcileSchedulerRBAC reconciles RBAC for the scheduler service.
func (r *Sandbox0InfraReconciler) reconcileSchedulerRBAC(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	name := fmt.Sprintf("%s-scheduler", infra.Name)
	labels := map[string]string{
		"app.kubernetes.io/name":       "scheduler",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}

	return r.reconcileServiceAccount(ctx, infra, name, labels)
}

// reconcileNetdRBAC reconciles RBAC for the netd service.
func (r *Sandbox0InfraReconciler) reconcileNetdRBAC(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	name := fmt.Sprintf("%s-netd", infra.Name)
	labels := map[string]string{
		"app.kubernetes.io/name":       "netd",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}

	if err := r.reconcileServiceAccount(ctx, infra, name, labels); err != nil {
		return err
	}

	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "nodes", "events"},
			Verbs:     []string{"get", "list", "watch", "create", "patch"},
		},
	}

	if err := r.reconcileClusterRole(ctx, infra, name, labels, rules); err != nil {
		return err
	}

	return r.reconcileClusterRoleBinding(ctx, infra, name, labels, name, name)
}

// reconcileStorageProxyRBAC reconciles RBAC for the storage-proxy service.
func (r *Sandbox0InfraReconciler) reconcileStorageProxyRBAC(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	name := fmt.Sprintf("%s-storage-proxy", infra.Name)
	labels := map[string]string{
		"app.kubernetes.io/name":       "storage-proxy",
		"app.kubernetes.io/instance":   infra.Name,
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}

	if err := r.reconcileServiceAccount(ctx, infra, name, labels); err != nil {
		return err
	}

	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "events"},
			Verbs:     []string{"get", "list", "watch", "create", "patch"},
		},
	}

	if err := r.reconcileClusterRole(ctx, infra, name, labels, rules); err != nil {
		return err
	}

	return r.reconcileClusterRoleBinding(ctx, infra, name, labels, name, name)
}
