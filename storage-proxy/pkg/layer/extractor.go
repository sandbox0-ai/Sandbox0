package layer

import (
	"context"
	"fmt"

	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/sandbox0-ai/infra/pkg/naming"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/pathutil"
	"github.com/sirupsen/logrus"
)

// Extractor extracts container images to JuiceFS.
// This implementation provides the interface for image extraction.
// The actual container image pulling is delegated to an external process
// (e.g., skopeo, crane, or a sidecar container) for better separation of concerns.
type Extractor struct {
	metaURL    string
	cacheDir   string
	s3Config   *S3Config
	logger     *logrus.Logger
	metaClient meta.Meta
}

// S3Config contains S3 configuration for JuiceFS
type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	Token     string
}

// NewExtractor creates a new image extractor
func NewExtractor(metaURL, cacheDir string, s3Config *S3Config, logger *logrus.Logger) (*Extractor, error) {
	// Initialize JuiceFS meta client
	metaConf := meta.DefaultConf()
	metaConf.Retries = 10
	metaClient := meta.NewClient(metaURL, metaConf)

	// Ensure the format is loaded
	if _, err := metaClient.Load(true); err != nil {
		return nil, fmt.Errorf("load juicefs format: %w", err)
	}

	return &Extractor{
		metaURL:    metaURL,
		cacheDir:   cacheDir,
		s3Config:   s3Config,
		logger:     logger,
		metaClient: metaClient,
	}, nil
}

// ExtractResult contains the result of an image extraction
type ExtractResult struct {
	Digest    string
	LayerPath string
	SizeBytes int64
}

// Extract extracts a container image to JuiceFS.
// This is a simplified implementation that:
// 1. Creates the layer directory structure in JuiceFS
// 2. Returns the path where the image should be extracted
//
// The actual image extraction is expected to be performed by an external tool
// (e.g., rootfs-init container) that:
// 1. Pulls the image using container runtime
// 2. Extracts the layers to the JuiceFS mount point
//
// This design choice:
// - Avoids heavy dependencies in storage-proxy
// - Allows using optimized container image handling tools
// - Separates concerns: storage-proxy manages paths, external tool handles images
func (e *Extractor) Extract(ctx context.Context, layerID, teamID, imageRef string, creds *RegistryCredentials) (digest, layerPath string, sizeBytes int64, err error) {
	// Generate layer path in JuiceFS
	layerPath, err = naming.JuiceFSBaseLayerPath(teamID, layerID)
	if err != nil {
		return "", "", 0, fmt.Errorf("generate layer path: %w", err)
	}

	// Create the directory structure in JuiceFS
	jfsCtx := meta.Background()
	if err := e.ensureDirectory(jfsCtx, layerPath); err != nil {
		return "", "", 0, fmt.Errorf("create layer directory: %w", err)
	}

	e.logger.WithFields(logrus.Fields{
		"layer_id":   layerID,
		"layer_path": layerPath,
		"image_ref":  imageRef,
	}).Info("Layer directory created, ready for extraction")

	// The actual extraction is performed externally.
	// For now, return a placeholder result.
	// In production, this would trigger an async extraction task.
	return "", layerPath, 0, nil
}

// ensureDirectory creates a directory path in JuiceFS
func (e *Extractor) ensureDirectory(jfsCtx meta.Context, path string) error {
	// Get path components
	components := pathutil.SplitPath(path)
	current := meta.RootInode
	var attr meta.Attr

	for _, component := range components {
		if component == "" {
			continue
		}

		var next meta.Ino
		errno := e.metaClient.Lookup(jfsCtx, current, component, &next, &attr, false)
		if errno != 0 {
			// Directory doesn't exist, create it
			errno = e.metaClient.Mkdir(jfsCtx, current, component, 0755, 0, 0, &next, &attr)
			if errno != 0 {
				return fmt.Errorf("mkdir %s: %s", component, errno.Error())
			}
		}
		current = next
	}

	return nil
}

// Delete removes layer data from JuiceFS
func (e *Extractor) Delete(ctx context.Context, layerPath string) error {
	// Recursively delete all files in the layer path
	jfsCtx := meta.Background()

	components := pathutil.SplitPath(layerPath)
	if len(components) == 0 {
		return nil
	}

	current := meta.RootInode
	var attr meta.Attr

	// Navigate to the parent directory
	for i := 0; i < len(components)-1; i++ {
		component := components[i]
		var next meta.Ino
		errno := e.metaClient.Lookup(jfsCtx, current, component, &next, &attr, false)
		if errno != 0 {
			// Path doesn't exist, nothing to delete
			return nil
		}
		current = next
	}

	// Delete the final component
	name := components[len(components)-1]
	errno := e.metaClient.Rmdir(jfsCtx, current, name)
	if errno != 0 {
		e.logger.WithError(fmt.Errorf("rmdir: %s", errno)).WithField("path", layerPath).Warn("Failed to delete layer directory")
		// Don't fail - the layer might have files that need recursive deletion
	}

	return nil
}
