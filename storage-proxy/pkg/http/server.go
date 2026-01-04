package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	pb "github.com/sandbox0/storage-proxy/proto/fs"
	"github.com/sandbox0/storage-proxy/pkg/volume"
	"github.com/sirupsen/logrus"
)

// Server provides HTTP management API
type Server struct {
	volMgr *volume.Manager
	logger *logrus.Logger
	mux    *http.ServeMux
}

// NewServer creates a new HTTP server
func NewServer(volMgr *volume.Manager, logger *logrus.Logger) *Server {
	s := &Server{
		volMgr: volMgr,
		logger: logger,
		mux:    http.NewServeMux(),
	}

	// Register handlers
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)
	s.mux.Handle("/metrics", promhttp.Handler())
	s.mux.HandleFunc("/api/v1/volumes", s.handleListVolumes)
	s.mux.HandleFunc("/api/v1/volumes/", s.handleVolumeOperations)

	return s
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"timestamp": time.Now().Unix(),
	})
}

// handleReady handles readiness check requests
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// Check if we can accept new volumes
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ready",
		"timestamp": time.Now().Unix(),
	})
}

// handleListVolumes handles volume list requests
func (s *Server) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	volumes := s.volMgr.ListVolumes()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"volumes": volumes,
		"count":   len(volumes),
	})
}

// handleVolumeOperations handles volume-specific operations
func (s *Server) handleVolumeOperations(w http.ResponseWriter, r *http.Request) {
	// Parse volume ID from path
	path := r.URL.Path
	// Example: /api/v1/volumes/{volume_id}/mount
	
	w.Header().Set("Content-Type", "application/json")
	
	switch r.Method {
	case http.MethodGet:
		s.handleGetVolume(w, r)
	case http.MethodPost:
		s.handleVolumeAction(w, r)
	case http.MethodDelete:
		s.handleUnmountVolume(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetVolume handles get volume info requests
func (s *Server) handleGetVolume(w http.ResponseWriter, r *http.Request) {
	// Extract volume ID from path (simplified)
	volumeID := extractVolumeID(r.URL.Path)
	if volumeID == "" {
		http.Error(w, "Invalid volume ID", http.StatusBadRequest)
		return
	}

	volCtx, err := s.volMgr.GetVolume(volumeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"volume_id":   volCtx.VolumeID,
		"is_mounted":  true,
		"mounted_at":  volCtx.MountedAt.Unix(),
		"read_only":   volCtx.Config.ReadOnly,
	})
}

// handleVolumeAction handles mount/unmount actions
func (s *Server) handleVolumeAction(w http.ResponseWriter, r *http.Request) {
	// Simplified - in production, parse full request
	var req struct {
		Action string                `json:"action"`
		Config *pb.VolumeConfig      `json:"config,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	volumeID := extractVolumeID(r.URL.Path)
	if volumeID == "" {
		http.Error(w, "Invalid volume ID", http.StatusBadRequest)
		return
	}

	switch req.Action {
	case "mount":
		if req.Config == nil {
			http.Error(w, "Missing config", http.StatusBadRequest)
			return
		}
		
		config := &volume.VolumeConfig{
			MetaURL:        req.Config.MetaUrl,
			S3Bucket:       req.Config.S3Bucket,
			S3Prefix:       req.Config.S3Prefix,
			S3Region:       req.Config.S3Region,
			S3Endpoint:     req.Config.S3Endpoint,
			S3AccessKey:    req.Config.S3AccessKey,
			S3SecretKey:    req.Config.S3SecretKey,
			S3SessionToken: req.Config.S3SessionToken,
			CacheDir:       req.Config.CacheDir,
			CacheSize:      req.Config.CacheSize,
			Prefetch:       int(req.Config.Prefetch),
			BufferSize:     req.Config.BufferSize,
			Writeback:      req.Config.Writeback,
			ReadOnly:       req.Config.ReadOnly,
		}

		err := s.volMgr.MountVolume(r.Context(), volumeID, config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"volume_id":  volumeID,
			"mounted_at": time.Now().Unix(),
		})

	case "unmount":
		err := s.volMgr.UnmountVolume(r.Context(), volumeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
	}
}

// handleUnmountVolume handles unmount requests
func (s *Server) handleUnmountVolume(w http.ResponseWriter, r *http.Request) {
	volumeID := extractVolumeID(r.URL.Path)
	if volumeID == "" {
		http.Error(w, "Invalid volume ID", http.StatusBadRequest)
		return
	}

	err := s.volMgr.UnmountVolume(r.Context(), volumeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// extractVolumeID extracts volume ID from URL path
// Example: /api/v1/volumes/vol-123/mount -> vol-123
func extractVolumeID(path string) string {
	// Simplified parsing - in production use a router like gorilla/mux
	// Path format: /api/v1/volumes/{volume_id}/...
	parts := splitPath(path)
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "volumes" {
		return parts[3]
	}
	return ""
}

func splitPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}

