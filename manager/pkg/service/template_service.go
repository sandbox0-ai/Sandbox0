package service

import (
	"context"
	"fmt"

	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	"github.com/sandbox0-ai/infra/manager/pkg/controller"
	clientset "github.com/sandbox0-ai/infra/manager/pkg/generated/clientset/versioned"
	"github.com/sandbox0-ai/infra/manager/pkg/network"
	"github.com/sandbox0-ai/infra/pkg/naming"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
)

// TemplateService handles template operations
type TemplateService struct {
	k8sClient         kubernetes.Interface
	crdClient         clientset.Interface
	templateLister    controller.TemplateLister
	namespaceLister   corelisters.NamespaceLister
	logger            *zap.Logger
	network           network.Provider
}

// NewTemplateService creates a new TemplateService
func NewTemplateService(
	k8sClient kubernetes.Interface,
	crdClient clientset.Interface,
	templateLister controller.TemplateLister,
	namespaceLister corelisters.NamespaceLister,
	networkProvider network.Provider,
	logger *zap.Logger,
) *TemplateService {
	if networkProvider == nil {
		networkProvider = network.NewNoopProvider()
	}
	return &TemplateService{
		k8sClient:         k8sClient,
		crdClient:         crdClient,
		templateLister:    templateLister,
		namespaceLister:   namespaceLister,
		logger:            logger,
		network:           networkProvider,
	}
}

// CreateTemplate creates a new template
func (s *TemplateService) CreateTemplate(ctx context.Context, template *v1alpha1.SandboxTemplate) (*v1alpha1.SandboxTemplate, error) {
	s.logger.Info("Creating template", zap.String("name", template.Name))

	namespace, err := naming.TemplateNamespaceFromName(template.Name)
	if err != nil {
		return nil, fmt.Errorf("resolve template namespace: %w", err)
	}
	template.Namespace = namespace

	if err := s.ensureNamespace(ctx, namespace); err != nil {
		return nil, err
	}

	if s.network != nil {
		if err := s.network.EnsureBaseline(ctx, namespace); err != nil {
			s.logger.Warn("Network provider baseline failed",
				zap.String("provider", s.network.Name()),
				zap.String("namespace", namespace),
				zap.Error(err),
			)
		}
	}

	// Set default values if needed
	if template.Spec.Pool.MinIdle < 0 {
		template.Spec.Pool.MinIdle = 0
	}
	if template.Spec.Pool.MaxIdle < template.Spec.Pool.MinIdle {
		template.Spec.Pool.MaxIdle = template.Spec.Pool.MinIdle
	}

	result, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(namespace).Create(ctx, template, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}

	return result, nil
}

// GetTemplate gets a template by ID (name) from the configured namespace.
func (s *TemplateService) GetTemplate(ctx context.Context, id string) (*v1alpha1.SandboxTemplate, error) {
	namespace, err := naming.TemplateNamespaceFromName(id)
	if err != nil {
		return nil, fmt.Errorf("resolve template namespace: %w", err)
	}
	template, err := s.templateLister.Get(namespace, id)
	if err != nil {
		return nil, err
	}
	return template, nil
}

// ListTemplates lists templates across namespaces.
func (s *TemplateService) ListTemplates(ctx context.Context) ([]*v1alpha1.SandboxTemplate, error) {
	templates, err := s.templateLister.List()
	if err != nil {
		return nil, err
	}
	return templates, nil
}

// UpdateTemplate updates an existing template
func (s *TemplateService) UpdateTemplate(ctx context.Context, template *v1alpha1.SandboxTemplate) (*v1alpha1.SandboxTemplate, error) {
	s.logger.Info("Updating template", zap.String("name", template.Name))

	namespace, err := naming.TemplateNamespaceFromName(template.Name)
	if err != nil {
		return nil, fmt.Errorf("resolve template namespace: %w", err)
	}
	template.Namespace = namespace

	// Helper to get current version for optimistic locking
	current, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(namespace).Get(ctx, template.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get current template: %w", err)
	}

	template.ResourceVersion = current.ResourceVersion

	// Preserve status
	template.Status = current.Status

	result, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(namespace).Update(ctx, template, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}

	return result, nil
}

// DeleteTemplate deletes a template from the configured namespace.
func (s *TemplateService) DeleteTemplate(ctx context.Context, id string) error {
	s.logger.Info("Deleting template", zap.String("name", id))

	namespace, err := naming.TemplateNamespaceFromName(id)
	if err != nil {
		return fmt.Errorf("resolve template namespace: %w", err)
	}
	err = s.crdClient.Sandbox0V1alpha1().SandboxTemplates(namespace).Delete(ctx, id, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("delete template: %w", err)
	}

	return nil
}

// WarmPool triggers pool warming for a template in the configured namespace.
func (s *TemplateService) WarmPool(ctx context.Context, id string, count int32) error {
	s.logger.Info("Warming pool", zap.String("name", id), zap.Int32("count", count))

	// Get current template
	namespace, err := naming.TemplateNamespaceFromName(id)
	if err != nil {
		return fmt.Errorf("resolve template namespace: %w", err)
	}
	template, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(namespace).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get template: %w", err)
	}

	// Update MinIdle if needed
	if template.Spec.Pool.MinIdle < count {
		template.Spec.Pool.MinIdle = count
		if template.Spec.Pool.MaxIdle < count {
			template.Spec.Pool.MaxIdle = count
		}

		_, err = s.crdClient.Sandbox0V1alpha1().SandboxTemplates(namespace).Update(ctx, template, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update template pool settings: %w", err)
		}
	}

	return nil
}

func (s *TemplateService) ensureNamespace(ctx context.Context, namespace string) error {
	if s.k8sClient == nil {
		return fmt.Errorf("k8s client is required to ensure namespace %s", namespace)
	}
	if s.namespaceLister == nil {
		return fmt.Errorf("namespace lister is required to ensure namespace %s", namespace)
	}

	if _, err := s.namespaceLister.Get(namespace); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("get namespace %s from cache: %w", namespace, err)
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "sandbox0-manager",
			},
		},
	}
	if _, err := s.k8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", namespace, err)
	}
	return nil
}
