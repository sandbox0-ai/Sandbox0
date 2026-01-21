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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sandbox0-ai/infra/infra-operator/api/v1alpha1"
)

const (
	controlPlaneKeySecretName = "sandbox0-internal-jwt-control-plane"
	dataPlaneKeySecretName    = "sandbox0-internal-jwt-data-plane"
)

// reconcileInternalAuth reconciles internal authentication keys
func (r *Sandbox0InfraReconciler) reconcileInternalAuth(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	switch infra.Spec.Mode {
	case infrav1alpha1.DeploymentModeAll:
		// Generate both control plane and data plane keys
		if err := r.reconcileControlPlaneKeys(ctx, infra); err != nil {
			return err
		}
		if err := r.reconcileDataPlaneKeys(ctx, infra); err != nil {
			return err
		}

	case infrav1alpha1.DeploymentModeControlPlane:
		// Only generate control plane keys
		if err := r.reconcileControlPlaneKeys(ctx, infra); err != nil {
			return err
		}

	case infrav1alpha1.DeploymentModeDataPlane:
		// Only generate data plane keys
		if err := r.reconcileDataPlaneKeys(ctx, infra); err != nil {
			return err
		}
	}

	// Update status with key locations
	r.updateInternalAuthStatus(ctx, infra)

	logger.Info("Internal auth keys reconciled successfully")
	return nil
}

// reconcileControlPlaneKeys creates or updates control plane key pair
func (r *Sandbox0InfraReconciler) reconcileControlPlaneKeys(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	secretName := fmt.Sprintf("%s-%s", infra.Name, controlPlaneKeySecretName)

	// Check if user provided existing secret
	if infra.Spec.InternalAuth != nil &&
		infra.Spec.InternalAuth.ControlPlane != nil &&
		infra.Spec.InternalAuth.ControlPlane.SecretRef != nil &&
		!infra.Spec.InternalAuth.ControlPlane.Generate {
		// Use existing secret, just validate it exists
		secret := &corev1.Secret{}
		return r.Get(ctx, types.NamespacedName{
			Name:      infra.Spec.InternalAuth.ControlPlane.SecretRef.Name,
			Namespace: infra.Namespace,
		}, secret)
	}

	return r.createKeyPairSecret(ctx, infra, secretName)
}

// reconcileDataPlaneKeys creates or updates data plane key pair
func (r *Sandbox0InfraReconciler) reconcileDataPlaneKeys(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	secretName := fmt.Sprintf("%s-%s", infra.Name, dataPlaneKeySecretName)

	// Check if user provided existing secret
	if infra.Spec.InternalAuth != nil &&
		infra.Spec.InternalAuth.DataPlane != nil &&
		infra.Spec.InternalAuth.DataPlane.SecretRef != nil &&
		!infra.Spec.InternalAuth.DataPlane.Generate {
		// Use existing secret, just validate it exists
		secret := &corev1.Secret{}
		return r.Get(ctx, types.NamespacedName{
			Name:      infra.Spec.InternalAuth.DataPlane.SecretRef.Name,
			Namespace: infra.Namespace,
		}, secret)
	}

	return r.createKeyPairSecret(ctx, infra, secretName)
}

// createKeyPairSecret creates an Ed25519 key pair and stores it in a secret
func (r *Sandbox0InfraReconciler) createKeyPairSecret(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, secretName string) error {
	logger := log.FromContext(ctx)

	// Check if secret already exists
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: infra.Namespace}, existingSecret)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Secret already exists, don't regenerate keys
	if err == nil {
		logger.Info("Key pair secret already exists", "secretName", secretName)
		return nil
	}

	// Generate new Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}

	// Encode keys to PEM format
	privateKeyPEM, err := encodeEd25519PrivateKeyToPEM(privateKey)
	if err != nil {
		return fmt.Errorf("failed to encode private key to PEM: %w", err)
	}
	publicKeyPEM, err := encodeEd25519PublicKeyToPEM(publicKey)
	if err != nil {
		return fmt.Errorf("failed to encode public key to PEM: %w", err)
	}

	// Create secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: infra.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "sandbox0-internal-auth",
				"app.kubernetes.io/instance":   infra.Name,
				"app.kubernetes.io/managed-by": "sandbox0infra-operator",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"private.key": privateKeyPEM,
			"public.key":  publicKeyPEM,
		},
	}

	if err := ctrl.SetControllerReference(infra, secret, r.Scheme); err != nil {
		return err
	}

	logger.Info("Creating key pair secret", "secretName", secretName)
	return r.Create(ctx, secret)
}

// encodeEd25519PrivateKeyToPEM encodes an Ed25519 private key to PEM format
func encodeEd25519PrivateKeyToPEM(privateKey ed25519.PrivateKey) ([]byte, error) {
	bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{
		Type:  "ED25519 PRIVATE KEY",
		Bytes: bytes,
	}
	return pem.EncodeToMemory(block), nil
}

// encodeEd25519PublicKeyToPEM encodes an Ed25519 public key to PEM format
func encodeEd25519PublicKeyToPEM(publicKey ed25519.PublicKey) ([]byte, error) {
	bytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{
		Type:  "ED25519 PUBLIC KEY",
		Bytes: bytes,
	}
	return pem.EncodeToMemory(block), nil
}

// updateInternalAuthStatus updates the internal auth status
func (r *Sandbox0InfraReconciler) updateInternalAuthStatus(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) {
	if infra.Status.InternalAuth == nil {
		infra.Status.InternalAuth = &infrav1alpha1.InternalAuthStatus{}
	}

	switch infra.Spec.Mode {
	case infrav1alpha1.DeploymentModeAll:
		controlPlaneSecret, _, controlPlanePublicKey := r.getControlPlaneKeyRefs(infra)
		dataPlaneSecret, _, dataPlanePublicKey := r.getDataPlaneKeyRefs(infra)
		infra.Status.InternalAuth.ControlPlanePublicKey = &infrav1alpha1.SecretKeyStatus{
			SecretName: controlPlaneSecret,
			SecretKey:  controlPlanePublicKey,
		}
		infra.Status.InternalAuth.DataPlanePublicKey = &infrav1alpha1.SecretKeyStatus{
			SecretName: dataPlaneSecret,
			SecretKey:  dataPlanePublicKey,
		}

	case infrav1alpha1.DeploymentModeControlPlane:
		secretName, _, publicKey := r.getControlPlaneKeyRefs(infra)
		infra.Status.InternalAuth.ControlPlanePublicKey = &infrav1alpha1.SecretKeyStatus{
			SecretName: secretName,
			SecretKey:  publicKey,
		}

	case infrav1alpha1.DeploymentModeDataPlane:
		secretName, _, publicKey := r.getDataPlaneKeyRefs(infra)
		infra.Status.InternalAuth.DataPlanePublicKey = &infrav1alpha1.SecretKeyStatus{
			SecretName: secretName,
			SecretKey:  publicKey,
		}
	}
}

func (r *Sandbox0InfraReconciler) getControlPlaneKeyRefs(infra *infrav1alpha1.Sandbox0Infra) (secretName, privateKeyKey, publicKeyKey string) {
	privateKeyKey = "private.key"
	publicKeyKey = "public.key"
	secretName = fmt.Sprintf("%s-%s", infra.Name, controlPlaneKeySecretName)

	if infra.Spec.InternalAuth != nil && infra.Spec.InternalAuth.ControlPlane != nil && infra.Spec.InternalAuth.ControlPlane.SecretRef != nil {
		ref := infra.Spec.InternalAuth.ControlPlane.SecretRef
		if ref.Name != "" {
			secretName = ref.Name
		}
		if ref.PrivateKeyKey != "" {
			privateKeyKey = ref.PrivateKeyKey
		}
		if ref.PublicKeyKey != "" {
			publicKeyKey = ref.PublicKeyKey
		}
	}

	return secretName, privateKeyKey, publicKeyKey
}

func (r *Sandbox0InfraReconciler) getDataPlaneKeyRefs(infra *infrav1alpha1.Sandbox0Infra) (secretName, privateKeyKey, publicKeyKey string) {
	privateKeyKey = "private.key"
	publicKeyKey = "public.key"
	secretName = fmt.Sprintf("%s-%s", infra.Name, dataPlaneKeySecretName)

	if infra.Spec.InternalAuth != nil && infra.Spec.InternalAuth.DataPlane != nil && infra.Spec.InternalAuth.DataPlane.SecretRef != nil {
		ref := infra.Spec.InternalAuth.DataPlane.SecretRef
		if ref.Name != "" {
			secretName = ref.Name
		}
		if ref.PrivateKeyKey != "" {
			privateKeyKey = ref.PrivateKeyKey
		}
		if ref.PublicKeyKey != "" {
			publicKeyKey = ref.PublicKeyKey
		}
	}

	return secretName, privateKeyKey, publicKeyKey
}

func (r *Sandbox0InfraReconciler) getControlPlanePublicKeyRef(infra *infrav1alpha1.Sandbox0Infra) (secretName, publicKeyKey string) {
	if infra.Spec.ControlPlane == nil {
		return "", ""
	}

	secretName = infra.Spec.ControlPlane.InternalAuthPublicKeySecret.Name
	publicKeyKey = infra.Spec.ControlPlane.InternalAuthPublicKeySecret.Key
	if publicKeyKey == "" || publicKeyKey == "password" {
		publicKeyKey = "public.key"
	}

	return secretName, publicKeyKey
}

// getControlPlaneKeySecret returns the control plane key secret name
func (r *Sandbox0InfraReconciler) getControlPlaneKeySecret(infra *infrav1alpha1.Sandbox0Infra) string {
	if infra.Spec.InternalAuth != nil &&
		infra.Spec.InternalAuth.ControlPlane != nil &&
		infra.Spec.InternalAuth.ControlPlane.SecretRef != nil {
		return infra.Spec.InternalAuth.ControlPlane.SecretRef.Name
	}
	return fmt.Sprintf("%s-%s", infra.Name, controlPlaneKeySecretName)
}

// getDataPlaneKeySecret returns the data plane key secret name
func (r *Sandbox0InfraReconciler) getDataPlaneKeySecret(infra *infrav1alpha1.Sandbox0Infra) string {
	if infra.Spec.InternalAuth != nil &&
		infra.Spec.InternalAuth.DataPlane != nil &&
		infra.Spec.InternalAuth.DataPlane.SecretRef != nil {
		return infra.Spec.InternalAuth.DataPlane.SecretRef.Name
	}
	return fmt.Sprintf("%s-%s", infra.Name, dataPlaneKeySecretName)
}

// getControlPlanePublicKeySecret returns the control plane public key secret name for data plane mode
func (r *Sandbox0InfraReconciler) getControlPlanePublicKeySecret(infra *infrav1alpha1.Sandbox0Infra) string {
	if infra.Spec.ControlPlane != nil {
		return infra.Spec.ControlPlane.InternalAuthPublicKeySecret.Name
	}
	return ""
}
