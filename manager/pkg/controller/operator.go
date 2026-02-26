package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sandbox0-ai/infra/infra-operator/api/config"
	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	obsmetrics "github.com/sandbox0-ai/infra/pkg/observability/metrics"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const (
	maxRetries = 5
)

// Operator is the main controller for SandboxTemplate CRD
type Operator struct {
	k8sClient      kubernetes.Interface
	podLister      corelisters.PodLister
	podsSynced     cache.InformerSynced
	poolManager    *PoolManager
	autoScaler     *AutoScaler
	recorder       record.EventRecorder
	clock          TimeProvider
	logger         *zap.Logger
	statsPublisher TemplateStatsPublisher

	workqueue workqueue.TypedRateLimitingInterface[string]

	metrics *obsmetrics.ManagerMetrics

	// Template informer and lister (to be injected)
	templateInformer cache.SharedIndexInformer
	templateLister   TemplateListerImpl

	// LayerClient for baselayer management
	layerClient LayerClient

	statsMu   sync.Mutex
	lastStats map[string]TemplateCounts
}

// LayerClient defines the interface for layer service operations
type LayerClient interface {
	EnsureBaseLayer(ctx context.Context, teamID, imageRef string) (*BaseLayerInfo, error)
	IncrementRefCount(ctx context.Context, teamID, layerID string) (int32, error)
	DecrementRefCount(ctx context.Context, teamID, layerID string) (int32, error)
}

// BaseLayerInfo contains base layer information
type BaseLayerInfo struct {
	ID       string
	Status   string
	Error    string
	ImageRef string
}

// TemplateListerImpl implements TemplateLister
type TemplateListerImpl struct {
	indexer cache.Indexer
}

// List lists all SandboxTemplates
func (t *TemplateListerImpl) List() ([]*v1alpha1.SandboxTemplate, error) {
	var templates []*v1alpha1.SandboxTemplate
	for _, obj := range t.indexer.List() {
		template := obj.(*v1alpha1.SandboxTemplate)
		templates = append(templates, template)
	}
	return templates, nil
}

// Get gets a SandboxTemplate by namespace and name
func (t *TemplateListerImpl) Get(namespace, name string) (*v1alpha1.SandboxTemplate, error) {
	key := namespace + "/" + name
	obj, exists, err := t.indexer.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("sandboxtemplate"), name)
	}
	return obj.(*v1alpha1.SandboxTemplate), nil
}

// NewOperator creates a new Operator
func NewOperator(
	k8sClient kubernetes.Interface,
	podInformer cache.SharedIndexInformer,
	replicaSetInformer cache.SharedIndexInformer,
	secretInformer cache.SharedIndexInformer,
	templateInformer cache.SharedIndexInformer,
	recorder record.EventRecorder,
	clock TimeProvider,
	logger *zap.Logger,
	metrics *obsmetrics.ManagerMetrics,
	autoscalerConfig config.AutoscalerConfig,
) *Operator {
	// Use system time as fallback if clock is nil
	if clock == nil {
		clock = systemTime{}
	}

	podLister := corelisters.NewPodLister(podInformer.GetIndexer())
	replicaSetLister := appslisters.NewReplicaSetLister(replicaSetInformer.GetIndexer())
	secretLister := corelisters.NewSecretLister(secretInformer.GetIndexer())
	poolManager := NewPoolManager(k8sClient, replicaSetLister, secretLister, recorder, logger)
	autoScaler := NewAutoScalerWithConfig(k8sClient, podLister, replicaSetLister, logger, toAutoScaleConfig(autoscalerConfig))

	op := &Operator{
		k8sClient:        k8sClient,
		podLister:        podLister,
		podsSynced:       podInformer.HasSynced,
		poolManager:      poolManager,
		autoScaler:       autoScaler,
		recorder:         recorder,
		clock:            clock,
		logger:           logger,
		workqueue:        workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]()),
		metrics:          metrics,
		templateInformer: templateInformer,
		templateLister: TemplateListerImpl{
			indexer: templateInformer.GetIndexer(),
		},
		lastStats: make(map[string]TemplateCounts),
	}

	// Setup event handlers for SandboxTemplate
	templateInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.handleTemplateAdd,
		UpdateFunc: op.handleTemplateUpdate,
		DeleteFunc: op.handleTemplateDelete,
	})

	// Setup event handlers for Pods to refresh template stats on pod changes.
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    op.handlePodAdd,
		UpdateFunc: op.handlePodUpdate,
		DeleteFunc: op.handlePodDelete,
	})

	return op
}

// Run starts the operator
func (op *Operator) Run(ctx context.Context, workers int) error {
	defer runtime.HandleCrash()
	defer op.workqueue.ShutDown()

	op.logger.Info("Starting operator")

	// Wait for cache sync
	op.logger.Info("Waiting for informer caches to sync")
	if !cache.WaitForCacheSync(ctx.Done(), op.podsSynced, op.templateInformer.HasSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	op.logger.Info("Starting workers", zap.Int("count", workers))
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, op.runWorker, time.Second)
	}

	op.logger.Info("Operator started")
	<-ctx.Done()
	op.logger.Info("Shutting down operator")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the workqueue
func (op *Operator) runWorker(ctx context.Context) {
	for op.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler
func (op *Operator) processNextWorkItem(ctx context.Context) bool {
	key, shutdown := op.workqueue.Get()
	if shutdown {
		return false
	}

	err := func(key string) error {
		defer op.workqueue.Done(key)

		if err := op.syncHandler(ctx, key); err != nil {
			// Requeue the item if there's an error
			if op.workqueue.NumRequeues(key) < maxRetries {
				op.logger.Error("Error syncing template, requeueing",
					zap.String("key", key),
					zap.Error(err),
				)
				op.workqueue.AddRateLimited(key)
				return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
			}

			// Drop the item after max retries
			op.workqueue.Forget(key)
			runtime.HandleError(fmt.Errorf("dropping template %q out of the queue: %v", key, err))
			return nil
		}

		op.workqueue.Forget(key)
		return nil
	}(key)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler reconciles a single SandboxTemplate
func (op *Operator) syncHandler(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the template
	template, err := op.templateLister.Get(namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("template '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	op.logger.Debug("Reconciling template", zap.String("name", name))

	// Reconcile baselayer if rootfs is enabled
	if template.Spec.Rootfs != nil && template.Spec.Rootfs.Enabled {
		if err := op.reconcileBaseLayer(ctx, template); err != nil {
			return fmt.Errorf("reconcile baselayer: %w", err)
		}
	}

	// Reconcile the pool (ReplicaSet)
	if err := op.poolManager.ReconcilePool(ctx, template); err != nil {
		return fmt.Errorf("reconcile pool: %w", err)
	}

	// Update status
	if err := op.updateTemplateStatus(ctx, template); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// Scale down for idle templates (async, background operation)
	// Scale up is handled synchronously in SandboxService.OnColdClaim
	if template.Spec.Pool.AutoScale && op.autoScaler != nil {
		if err := op.autoScaler.ReconcileScaleDown(ctx, template, op.clock.Now()); err != nil {
			op.logger.Warn("Scale down reconcile failed",
				zap.String("template", template.Name),
				zap.String("namespace", template.Namespace),
				zap.Error(err),
			)
			// Don't fail the reconcile; pool + status are still correct.
		}
	}

	return nil
}

// updateTemplateStatus updates the status of a SandboxTemplate
func (op *Operator) updateTemplateStatus(ctx context.Context, template *v1alpha1.SandboxTemplate) error {
	// Get idle pods
	idlePods, err := op.podLister.Pods(template.ObjectMeta.Namespace).List(labels.SelectorFromSet(map[string]string{
		LabelTemplateID: template.ObjectMeta.Name,
		LabelPoolType:   PoolTypeIdle,
	}))
	if err != nil {
		return err
	}

	// Get active pods
	activePods, err := op.podLister.Pods(template.ObjectMeta.Namespace).List(labels.SelectorFromSet(map[string]string{
		LabelTemplateID: template.ObjectMeta.Name,
		LabelPoolType:   PoolTypeActive,
	}))
	if err != nil {
		return err
	}

	// Count running pods only
	idleCount := int32(0)
	for _, pod := range idlePods {
		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
			idleCount++
		}
	}

	activeCount := int32(0)
	for _, pod := range activePods {
		if pod.Status.Phase == corev1.PodRunning {
			activeCount++
		}
	}

	if op.metrics != nil {
		op.metrics.IdlePodsTotal.WithLabelValues(template.Name).Set(float64(idleCount))
		op.metrics.ActivePodsTotal.WithLabelValues(template.Name).Set(float64(activeCount))
	}

	// Publish stats if changed.
	statsKey := template.Namespace + "/" + template.Name
	shouldPublish := false
	op.statsMu.Lock()
	last := op.lastStats[statsKey]
	if last.IdleCount != idleCount || last.ActiveCount != activeCount {
		op.lastStats[statsKey] = TemplateCounts{
			IdleCount:   idleCount,
			ActiveCount: activeCount,
		}
		shouldPublish = true
	}
	op.statsMu.Unlock()

	if shouldPublish && op.statsPublisher != nil {
		if err := op.statsPublisher.PublishTemplateStats(ctx, template, idleCount, activeCount); err != nil {
			op.logger.Warn("Failed to publish template stats",
				zap.String("template", template.ObjectMeta.Name),
				zap.Error(err),
			)
		}
	}

	// Update status if changed
	if template.Status.IdleCount != idleCount || template.Status.ActiveCount != activeCount {
		template.Status.IdleCount = idleCount
		template.Status.ActiveCount = activeCount
		template.Status.LastUpdateTime = metav1.Now()

		// Update conditions
		template.Status.Conditions = op.computeConditions(template, idleCount, activeCount)

		// Note: In a real implementation, we should use a status subresource update
		// For now, we'll just log the status
		op.logger.Info("Template status updated",
			zap.String("template", template.ObjectMeta.Name),
			zap.Int32("idle", idleCount),
			zap.Int32("active", activeCount),
		)
	}

	return nil
}

// computeConditions computes the conditions for a template
func (op *Operator) computeConditions(template *v1alpha1.SandboxTemplate, idleCount, activeCount int32) []v1alpha1.SandboxTemplateCondition {
	conditions := []v1alpha1.SandboxTemplateCondition{}

	// Ready condition
	readyStatus := v1alpha1.ConditionTrue
	readyReason := "PoolReady"
	readyMessage := "Pool is ready"
	if idleCount < template.Spec.Pool.MinIdle {
		readyStatus = v1alpha1.ConditionFalse
		readyReason = "InsufficientIdlePods"
		readyMessage = fmt.Sprintf("Idle pod count (%d) is less than minIdle (%d)", idleCount, template.Spec.Pool.MinIdle)
	}

	conditions = append(conditions, v1alpha1.SandboxTemplateCondition{
		Type:               v1alpha1.SandboxTemplateReady,
		Status:             readyStatus,
		LastTransitionTime: metav1.Now(),
		Reason:             readyReason,
		Message:            readyMessage,
	})

	// PoolHealthy condition
	healthyStatus := v1alpha1.ConditionTrue
	healthyReason := "PoolHealthy"
	healthyMessage := "Pool is healthy"
	if idleCount > template.Spec.Pool.MaxIdle {
		healthyStatus = v1alpha1.ConditionFalse
		healthyReason = "ExcessIdlePods"
		healthyMessage = fmt.Sprintf("Idle pod count (%d) exceeds maxIdle (%d)", idleCount, template.Spec.Pool.MaxIdle)
	}

	conditions = append(conditions, v1alpha1.SandboxTemplateCondition{
		Type:               v1alpha1.SandboxTemplatePoolHealthy,
		Status:             healthyStatus,
		LastTransitionTime: metav1.Now(),
		Reason:             healthyReason,
		Message:            healthyMessage,
	})

	return conditions
}

// Event handlers

func (op *Operator) handleTemplateAdd(obj any) {
	template := obj.(*v1alpha1.SandboxTemplate)
	op.logger.Info("Template added", zap.String("name", template.ObjectMeta.Name))
	op.enqueueTemplate(template)
}

func (op *Operator) handleTemplateUpdate(oldObj, newObj any) {
	oldTemplate := oldObj.(*v1alpha1.SandboxTemplate)
	newTemplate := newObj.(*v1alpha1.SandboxTemplate)

	if oldTemplate.ObjectMeta.ResourceVersion == newTemplate.ObjectMeta.ResourceVersion {
		return
	}

	op.logger.Info("Template updated", zap.String("name", newTemplate.Name))
	op.enqueueTemplate(newTemplate)
}

func (op *Operator) handleTemplateDelete(obj any) {
	template, ok := obj.(*v1alpha1.SandboxTemplate)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		template, ok = tombstone.Obj.(*v1alpha1.SandboxTemplate)
		if !ok {
			runtime.HandleError(fmt.Errorf("tombstone contained object that is not a SandboxTemplate %#v", obj))
			return
		}
	}

	op.logger.Info("Template deleted", zap.String("name", template.ObjectMeta.Name))

	// Cleanup baselayer reference when template is deleted
	op.cleanupBaseLayerReference(context.Background(), template)

	// Cleanup is handled by owner references
}

func (op *Operator) enqueueTemplate(template *v1alpha1.SandboxTemplate) {
	key, err := cache.MetaNamespaceKeyFunc(template)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	op.workqueue.Add(key)
}

func (op *Operator) enqueueTemplateKey(namespace, name string) {
	key := namespace + "/" + name
	op.workqueue.Add(key)
}

func (op *Operator) handlePodAdd(obj any) {
	pod := obj.(*corev1.Pod)
	op.enqueueTemplateForPod(pod)
}

func (op *Operator) handlePodUpdate(oldObj, newObj any) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)
	if oldPod.ResourceVersion == newPod.ResourceVersion {
		return
	}
	op.enqueueTemplateForPod(newPod)
}

func (op *Operator) handlePodDelete(obj any) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("couldn't get pod from tombstone %#v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			runtime.HandleError(fmt.Errorf("tombstone contained object that is not a Pod %#v", obj))
			return
		}
	}
	op.enqueueTemplateForPod(pod)
}

func (op *Operator) enqueueTemplateForPod(pod *corev1.Pod) {
	if pod == nil || pod.Labels == nil {
		return
	}
	templateID := pod.Labels[LabelTemplateID]
	if templateID == "" {
		return
	}
	op.enqueueTemplateKey(pod.Namespace, templateID)
}

// GetTemplateLister returns the template lister
func (op *Operator) GetTemplateLister() TemplateLister {
	return &op.templateLister
}

// GetAutoScaler returns the sync scaler for use in sandbox service
func (op *Operator) GetAutoScaler() *AutoScaler {
	return op.autoScaler
}

// SetTemplateStatsPublisher injects a stats publisher (optional).
func (op *Operator) SetTemplateStatsPublisher(publisher TemplateStatsPublisher) {
	op.statsPublisher = publisher
}

// SetLayerClient injects a layer client for baselayer management (optional).
func (op *Operator) SetLayerClient(client LayerClient) {
	op.layerClient = client
}

// reconcileBaseLayer ensures a baselayer exists for the template's image reference.
// This method is called during template reconciliation when rootfs is enabled.
func (op *Operator) reconcileBaseLayer(ctx context.Context, template *v1alpha1.SandboxTemplate) error {
	if op.layerClient == nil {
		op.logger.Warn("LayerClient not configured, skipping baselayer reconciliation",
			zap.String("template", template.Name),
		)
		return nil
	}

	imageRef := template.Spec.MainContainer.Image
	if imageRef == "" {
		return fmt.Errorf("template %s has no main container image specified", template.Name)
	}

	teamID := op.getBaseLayerTeamID(template)

	// Check if image reference has changed
	storedImageRef := ""
	if template.Annotations != nil {
		storedImageRef = template.Annotations["sandbox0.ai/baselayer-image-ref"]
	}

	// If image changed and we had a previous baselayer, decrement its ref count
	if template.Spec.BaseLayerID != "" && storedImageRef != "" && storedImageRef != imageRef {
		op.logger.Info("Image reference changed, decrementing old baselayer ref count",
			zap.String("template", template.Name),
			zap.String("old_image_ref", storedImageRef),
			zap.String("new_image_ref", imageRef),
			zap.String("old_layer_id", template.Spec.BaseLayerID),
		)
		if _, err := op.layerClient.DecrementRefCount(ctx, teamID, template.Spec.BaseLayerID); err != nil {
			op.logger.Warn("Failed to decrement old baselayer ref count",
				zap.String("template", template.Name),
				zap.String("layer_id", template.Spec.BaseLayerID),
				zap.Error(err),
			)
			// Continue anyway - the new baselayer still needs to be set up
		}
		template.Spec.BaseLayerID = ""
	}

	// Ensure baselayer exists
	layerInfo, err := op.layerClient.EnsureBaseLayer(ctx, teamID, imageRef)
	if err != nil {
		op.logger.Error("Failed to ensure baselayer",
			zap.String("template", template.Name),
			zap.String("image_ref", imageRef),
			zap.Error(err),
		)
		return fmt.Errorf("ensure baselayer for image %s: %w", imageRef, err)
	}

	// Update template with baselayer info
	needsUpdate := false

	// First time associating this layer - increment ref count
	if template.Spec.BaseLayerID != layerInfo.ID {
		if _, err := op.layerClient.IncrementRefCount(ctx, teamID, layerInfo.ID); err != nil {
			return fmt.Errorf("increment baselayer ref count: %w", err)
		}
		template.Spec.BaseLayerID = layerInfo.ID
		needsUpdate = true
		op.logger.Info("Associated template with baselayer",
			zap.String("template", template.Name),
			zap.String("layer_id", layerInfo.ID),
			zap.String("image_ref", imageRef),
		)
	}

	// Update annotation for image reference tracking
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}
	if template.Annotations["sandbox0.ai/baselayer-image-ref"] != imageRef {
		template.Annotations["sandbox0.ai/baselayer-image-ref"] = imageRef
		needsUpdate = true
	}

	// Update status with baselayer info
	if template.Status.BaseLayerID != layerInfo.ID ||
		template.Status.BaseLayerStatus != layerInfo.Status ||
		template.Status.BaseLayerError != layerInfo.Error {
		template.Status.BaseLayerID = layerInfo.ID
		template.Status.BaseLayerStatus = layerInfo.Status
		template.Status.BaseLayerError = layerInfo.Error
		needsUpdate = true
	}

	// Update BaseLayerReady condition
	op.updateBaseLayerCondition(template, layerInfo.Status == v1alpha1.BaseLayerStatusReady, layerInfo.Error)

	if needsUpdate {
		op.logger.Debug("Template baselayer updated",
			zap.String("template", template.Name),
			zap.String("layer_id", layerInfo.ID),
			zap.String("status", layerInfo.Status),
		)
	}

	return nil
}

// getBaseLayerTeamID determines which team owns the baselayer for a template.
// Public templates and builtin templates use the system team ID.
func (op *Operator) getBaseLayerTeamID(template *v1alpha1.SandboxTemplate) string {
	// Builtin templates always use system team
	if template.Labels != nil && template.Labels["sandbox0.ai/template-scope"] == "builtin" {
		return v1alpha1.SystemTeamID
	}

	// Public templates use system team
	if template.Spec.Public {
		return v1alpha1.SystemTeamID
	}

	// Private templates use the team from annotation
	if template.Annotations != nil {
		if teamID := template.Annotations["sandbox0.ai/template-team-id"]; teamID != "" {
			return teamID
		}
	}

	// Default to system team for backward compatibility
	return v1alpha1.SystemTeamID
}

// updateBaseLayerCondition updates the BaseLayerReady condition
func (op *Operator) updateBaseLayerCondition(template *v1alpha1.SandboxTemplate, ready bool, errMsg string) {
	var status v1alpha1.ConditionStatus
	var reason, message string

	if ready {
		status = v1alpha1.ConditionTrue
		reason = "BaseLayerReady"
		message = "Base layer is ready for use"
	} else {
		status = v1alpha1.ConditionFalse
		if errMsg != "" {
			reason = "BaseLayerFailed"
			message = errMsg
		} else {
			reason = "BaseLayerExtracting"
			message = "Base layer is being extracted"
		}
	}

	// Check if condition already exists with same status
	for i, c := range template.Status.Conditions {
		if c.Type == v1alpha1.SandboxTemplateBaseLayerReady {
			if c.Status == status && c.Reason == reason && c.Message == message {
				return // No update needed
			}
			template.Status.Conditions[i] = v1alpha1.SandboxTemplateCondition{
				Type:               v1alpha1.SandboxTemplateBaseLayerReady,
				Status:             status,
				LastTransitionTime: metav1.Now(),
				Reason:             reason,
				Message:            message,
			}
			return
		}
	}

	// Add new condition
	template.Status.Conditions = append(template.Status.Conditions, v1alpha1.SandboxTemplateCondition{
		Type:               v1alpha1.SandboxTemplateBaseLayerReady,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// cleanupBaseLayerReference decrements the reference count for a template's baselayer.
// This should be called when a template is being deleted.
func (op *Operator) cleanupBaseLayerReference(ctx context.Context, template *v1alpha1.SandboxTemplate) {
	if op.layerClient == nil {
		return
	}

	if template.Spec.Rootfs == nil || !template.Spec.Rootfs.Enabled {
		return
	}

	if template.Spec.BaseLayerID == "" {
		return
	}

	teamID := op.getBaseLayerTeamID(template)
	if _, err := op.layerClient.DecrementRefCount(ctx, teamID, template.Spec.BaseLayerID); err != nil {
		op.logger.Warn("Failed to decrement baselayer ref count on template deletion",
			zap.String("template", template.Name),
			zap.String("layer_id", template.Spec.BaseLayerID),
			zap.Error(err),
		)
	} else {
		op.logger.Info("Decremented baselayer ref count on template deletion",
			zap.String("template", template.Name),
			zap.String("layer_id", template.Spec.BaseLayerID),
		)
	}
}

// toAutoScaleConfig converts config.AutoscalerConfig to AutoScaleConfig.
func toAutoScaleConfig(cfg config.AutoscalerConfig) AutoScaleConfig {
	defaultCfg := DefaultAutoScaleConfig()
	return AutoScaleConfig{
		MinScaleInterval:        cfg.MinScaleInterval.Duration,
		ScaleUpFactor:           cfg.ParsedScaleUpFactor(defaultCfg.ScaleUpFactor),
		MaxScaleStep:            cfg.MaxScaleStep,
		MinIdleBuffer:           cfg.MinIdleBuffer,
		TargetIdleRatio:         cfg.ParsedTargetIdleRatio(defaultCfg.TargetIdleRatio),
		NoTrafficScaleDownAfter: cfg.NoTrafficScaleDownAfter.Duration,
		ScaleDownPercent:        cfg.ParsedScaleDownPercent(defaultCfg.ScaleDownPercent),
	}
}
