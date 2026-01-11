package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/snapshot"
)

// CreateSnapshotRequest is the request body for creating a snapshot
type createSnapshotRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SnapshotResponse is the response for snapshot operations
type snapshotResponse struct {
	ID          string  `json:"id"`
	VolumeID    string  `json:"volume_id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	SizeBytes   int64   `json:"size_bytes"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
}

// createSnapshot handles POST /sandboxvolumes/{volume_id}/snapshots
func (s *Server) createSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	volumeID := r.PathValue("volume_id")
	if volumeID == "" {
		http.Error(w, "volume_id is required", http.StatusBadRequest)
		return
	}

	var req createSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	snap, err := s.snapshotMgr.CreateSnapshotSimple(r.Context(), &snapshot.CreateSnapshotRequest{
		VolumeID:    volumeID,
		Name:        req.Name,
		Description: req.Description,
		TeamID:      claims.TeamID,
		UserID:      claims.UserID,
	})

	if err != nil {
		s.handleSnapshotError(w, err)
		return
	}

	resp := snapshotResponse{
		ID:          snap.ID,
		VolumeID:    snap.VolumeID,
		Name:        snap.Name,
		Description: snap.Description,
		SizeBytes:   snap.SizeBytes,
		CreatedAt:   snap.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if snap.ExpiresAt != nil {
		expires := snap.ExpiresAt.Format("2006-01-02T15:04:05Z")
		resp.ExpiresAt = &expires
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// listSnapshots handles GET /sandboxvolumes/{volume_id}/snapshots
func (s *Server) listSnapshots(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	volumeID := r.PathValue("volume_id")
	if volumeID == "" {
		http.Error(w, "volume_id is required", http.StatusBadRequest)
		return
	}

	snapshots, err := s.snapshotMgr.ListSnapshots(r.Context(), volumeID, claims.TeamID)
	if err != nil {
		s.handleSnapshotError(w, err)
		return
	}

	var responses []snapshotResponse
	for _, snap := range snapshots {
		resp := snapshotResponse{
			ID:          snap.ID,
			VolumeID:    snap.VolumeID,
			Name:        snap.Name,
			Description: snap.Description,
			SizeBytes:   snap.SizeBytes,
			CreatedAt:   snap.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if snap.ExpiresAt != nil {
			expires := snap.ExpiresAt.Format("2006-01-02T15:04:05Z")
			resp.ExpiresAt = &expires
		}
		responses = append(responses, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// getSnapshot handles GET /sandboxvolumes/{volume_id}/snapshots/{snapshot_id}
func (s *Server) getSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	volumeID := r.PathValue("volume_id")
	snapshotID := r.PathValue("snapshot_id")
	if volumeID == "" || snapshotID == "" {
		http.Error(w, "volume_id and snapshot_id are required", http.StatusBadRequest)
		return
	}

	snap, err := s.snapshotMgr.GetSnapshot(r.Context(), volumeID, snapshotID, claims.TeamID)
	if err != nil {
		s.handleSnapshotError(w, err)
		return
	}

	resp := snapshotResponse{
		ID:          snap.ID,
		VolumeID:    snap.VolumeID,
		Name:        snap.Name,
		Description: snap.Description,
		SizeBytes:   snap.SizeBytes,
		CreatedAt:   snap.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if snap.ExpiresAt != nil {
		expires := snap.ExpiresAt.Format("2006-01-02T15:04:05Z")
		resp.ExpiresAt = &expires
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// restoreSnapshot handles POST /sandboxvolumes/{volume_id}/snapshots/{snapshot_id}/restore
func (s *Server) restoreSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	volumeID := r.PathValue("volume_id")
	snapshotID := r.PathValue("snapshot_id")
	if volumeID == "" || snapshotID == "" {
		http.Error(w, "volume_id and snapshot_id are required", http.StatusBadRequest)
		return
	}

	err := s.snapshotMgr.RestoreSnapshot(r.Context(), &snapshot.RestoreSnapshotRequest{
		VolumeID:   volumeID,
		SnapshotID: snapshotID,
		TeamID:     claims.TeamID,
		UserID:     claims.UserID,
	})

	if err != nil {
		s.handleSnapshotError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "restored"})
}

// deleteSnapshot handles DELETE /sandboxvolumes/{volume_id}/snapshots/{snapshot_id}
func (s *Server) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	volumeID := r.PathValue("volume_id")
	snapshotID := r.PathValue("snapshot_id")
	if volumeID == "" || snapshotID == "" {
		http.Error(w, "volume_id and snapshot_id are required", http.StatusBadRequest)
		return
	}

	err := s.snapshotMgr.DeleteSnapshot(r.Context(), volumeID, snapshotID, claims.TeamID)
	if err != nil {
		s.handleSnapshotError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleSnapshotError maps snapshot errors to HTTP responses
func (s *Server) handleSnapshotError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, snapshot.ErrVolumeNotFound):
		http.Error(w, "volume not found", http.StatusNotFound)
	case errors.Is(err, snapshot.ErrSnapshotNotFound):
		http.Error(w, "snapshot not found", http.StatusNotFound)
	case errors.Is(err, snapshot.ErrSnapshotNotBelongToVolume):
		http.Error(w, "snapshot not found", http.StatusNotFound) // Don't reveal existence
	case errors.Is(err, snapshot.ErrVolumeLocked):
		http.Error(w, "volume is locked for another operation", http.StatusConflict)
	case errors.Is(err, snapshot.ErrFlushFailed):
		http.Error(w, "failed to flush data", http.StatusInternalServerError)
	case errors.Is(err, snapshot.ErrCloneFailed):
		http.Error(w, "clone operation failed", http.StatusInternalServerError)
	default:
		s.logger.WithError(err).Error("Snapshot operation failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
