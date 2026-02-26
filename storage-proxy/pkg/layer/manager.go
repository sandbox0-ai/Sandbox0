package layer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sirupsen/logrus"
)

var (
	ErrLayerNotFound   = fmt.Errorf("base layer not found")
	ErrLayerInUse      = fmt.Errorf("base layer is in use")
	ErrLayerExtracting = fmt.Errorf("base layer is being extracted")
	ErrLayerFailed     = fmt.Errorf("base layer extraction failed")
)

// Manager manages base layer lifecycle
type Manager struct {
	repo    *db.Repository
	extract *Extractor
	logger  *logrus.Logger

	// Track ongoing extractions to prevent duplicates
	mu         sync.RWMutex
	extracting map[string]struct{} // teamID+imageRef -> struct{}
}

// NewManager creates a new layer manager
func NewManager(repo *db.Repository, extract *Extractor, logger *logrus.Logger) *Manager {
	return &Manager{
		repo:       repo,
		extract:    extract,
		logger:     logger,
		extracting: make(map[string]struct{}),
	}
}

// ExtractLayerRequest contains parameters for layer extraction
type ExtractLayerRequest struct {
	ID          string
	TeamID      string
	ImageRef    string
	Credentials *RegistryCredentials
	LayerName   string
}

// RegistryCredentials contains authentication for private registries
type RegistryCredentials struct {
	Username      string
	Password      string
	ServerAddress string
	Auth          string
	IdentityToken string
	RegistryToken string
}

// ExtractLayer starts extracting a base layer from an image registry
func (m *Manager) ExtractLayer(ctx context.Context, req *ExtractLayerRequest) (*db.BaseLayer, error) {
	// Check for existing layer with same image ref
	existing, err := m.repo.GetBaseLayerByImageRef(ctx, req.TeamID, req.ImageRef)
	if err == nil {
		// Layer exists, check status
		switch existing.Status {
		case db.BaseLayerStatusReady:
			return existing, nil
		case db.BaseLayerStatusExtracting:
			return existing, ErrLayerExtracting
		case db.BaseLayerStatusFailed:
			// Allow re-extraction of failed layers
		default:
			return existing, fmt.Errorf("layer in unexpected status: %s", existing.Status)
		}
	}

	// Check if extraction is already in progress
	extractionKey := req.TeamID + ":" + req.ImageRef
	m.mu.Lock()
	if _, exists := m.extracting[extractionKey]; exists {
		m.mu.Unlock()
		return nil, ErrLayerExtracting
	}
	m.extracting[extractionKey] = struct{}{}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.extracting, extractionKey)
		m.mu.Unlock()
	}()

	// Create layer record
	now := time.Now()
	layerID := req.ID
	if layerID == "" {
		layerID = uuid.New().String()
	}

	layer := &db.BaseLayer{
		ID:        layerID,
		TeamID:    req.TeamID,
		ImageRef:  req.ImageRef,
		Status:    db.BaseLayerStatusPending,
		RefCount:  0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.repo.CreateBaseLayer(ctx, layer); err != nil {
		return nil, fmt.Errorf("create base layer record: %w", err)
	}

	// Start async extraction
	go m.extractAsync(layer, req.Credentials)

	return layer, nil
}

// extractAsync performs the actual extraction in background
func (m *Manager) extractAsync(layer *db.BaseLayer, creds *RegistryCredentials) {
	ctx := context.Background()

	// Update status to extracting
	if err := m.repo.UpdateBaseLayerStatus(ctx, layer.ID, db.BaseLayerStatusExtracting, ""); err != nil {
		m.logger.WithError(err).WithField("layer_id", layer.ID).Error("Failed to update layer status to extracting")
		return
	}

	// Perform extraction
	digest, layerPath, sizeBytes, err := m.extract.Extract(ctx, layer.ID, layer.TeamID, layer.ImageRef, creds)
	if err != nil {
		m.logger.WithError(err).WithFields(logrus.Fields{
			"layer_id":  layer.ID,
			"image_ref": layer.ImageRef,
		}).Error("Layer extraction failed")

		if updateErr := m.repo.UpdateBaseLayerStatus(ctx, layer.ID, db.BaseLayerStatusFailed, err.Error()); updateErr != nil {
			m.logger.WithError(updateErr).Error("Failed to update layer status to failed")
		}
		return
	}

	// Update layer with extraction results
	if err := m.repo.UpdateBaseLayerExtraction(ctx, layer.ID, digest, layerPath, sizeBytes); err != nil {
		m.logger.WithError(err).WithField("layer_id", layer.ID).Error("Failed to update layer extraction results")
		return
	}

	m.logger.WithFields(logrus.Fields{
		"layer_id":   layer.ID,
		"image_ref":  layer.ImageRef,
		"size_bytes": sizeBytes,
	}).Info("Layer extraction completed")
}

// GetLayer retrieves a base layer by ID
func (m *Manager) GetLayer(ctx context.Context, id string) (*db.BaseLayer, error) {
	layer, err := m.repo.GetBaseLayer(ctx, id)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrLayerNotFound
		}
		return nil, fmt.Errorf("get base layer: %w", err)
	}
	return layer, nil
}

// GetLayerByImageRef retrieves a base layer by team and image reference
func (m *Manager) GetLayerByImageRef(ctx context.Context, teamID, imageRef string) (*db.BaseLayer, error) {
	layer, err := m.repo.GetBaseLayerByImageRef(ctx, teamID, imageRef)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrLayerNotFound
		}
		return nil, fmt.Errorf("get base layer by image ref: %w", err)
	}
	return layer, nil
}

// ListLayers lists base layers with optional filtering
func (m *Manager) ListLayers(ctx context.Context, teamID, status string, limit, offset int) ([]*db.BaseLayer, int, error) {
	return m.repo.ListBaseLayers(ctx, teamID, status, limit, offset)
}

// DeleteLayer deletes a base layer (only if not in use)
func (m *Manager) DeleteLayer(ctx context.Context, id string, force bool) error {
	layer, err := m.repo.GetBaseLayer(ctx, id)
	if err != nil {
		if err == db.ErrNotFound {
			return ErrLayerNotFound
		}
		return fmt.Errorf("get base layer: %w", err)
	}

	if !force && layer.RefCount > 0 {
		return ErrLayerInUse
	}

	// Delete the layer data from storage
	if layer.LayerPath != "" {
		if err := m.extract.Delete(ctx, layer.LayerPath); err != nil {
			m.logger.WithError(err).WithField("layer_path", layer.LayerPath).Warn("Failed to delete layer data")
			// Continue with database deletion
		}
	}

	return m.repo.DeleteBaseLayer(ctx, id)
}

// IncrementRefCount increments the reference count for a layer
func (m *Manager) IncrementRefCount(ctx context.Context, id string) (int, error) {
	return m.repo.IncrementBaseLayerRef(ctx, id)
}

// DecrementRefCount decrements the reference count for a layer
func (m *Manager) DecrementRefCount(ctx context.Context, id string) (int, error) {
	return m.repo.DecrementBaseLayerRef(ctx, id)
}

// GarbageCollect removes unused layers based on age
func (m *Manager) GarbageCollect(ctx context.Context, teamID string, minAgeSeconds, maxCount int, dryRun bool) ([]string, int64, []error) {
	layers, err := m.repo.ListUnusedBaseLayers(ctx, minAgeSeconds, maxCount)
	if err != nil {
		return nil, 0, []error{fmt.Errorf("list unused layers: %w", err)}
	}

	var deletedIDs []string
	var freedBytes int64
	var errs []error

	for _, layer := range layers {
		if dryRun {
			deletedIDs = append(deletedIDs, layer.ID)
			freedBytes += layer.SizeBytes
			continue
		}

		if err := m.DeleteLayer(ctx, layer.ID, false); err != nil {
			errs = append(errs, fmt.Errorf("delete layer %s: %w", layer.ID, err))
			continue
		}

		deletedIDs = append(deletedIDs, layer.ID)
		freedBytes += layer.SizeBytes
	}

	return deletedIDs, freedBytes, errs
}
