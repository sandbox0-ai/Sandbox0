package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/sandbox0-ai/infra/pkg/gateway/spec"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/rootfs"
)

// rootfsInfoResponse is the response for getRootfs
type rootfsInfoResponse struct {
	SandboxID     string `json:"sandbox_id"`
	TeamID        string `json:"team_id"`
	BaseLayerID   string `json:"base_layer_id"`
	UpperVolumeID string `json:"upper_volume_id"`
	UpperPath     string `json:"upper_path"`
	WorkPath      string `json:"work_path"`
	MountPath     string `json:"mount_path,omitempty"`
	Mounted       bool   `json:"mounted"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// createRootfsSnapshotRequest is the request body for creating a rootfs snapshot
type createRootfsSnapshotRequest struct {
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	RetentionSeconds int               `json:"retention_seconds,omitempty"`
}

// rootfsSnapshotResponse is the response for rootfs snapshot operations
type rootfsSnapshotResponse struct {
	ID            string            `json:"id"`
	SandboxID     string            `json:"sandbox_id"`
	TeamID        string            `json:"team_id"`
	BaseLayerID   string            `json:"base_layer_id"`
	UpperVolumeID string            `json:"upper_volume_id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	SizeBytes     int64             `json:"size_bytes"`
	RootInode     int64             `json:"root_inode"`
	SourceInode   int64             `json:"source_inode"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     string            `json:"created_at"`
	ExpiresAt     *string           `json:"expires_at,omitempty"`
}

// restoreRootfsSnapshotRequest is the request body for restoring a rootfs snapshot
type restoreRootfsSnapshotRequest struct {
	CreateBackup bool   `json:"create_backup,omitempty"`
	BackupName   string `json:"backup_name,omitempty"`
}

// forkRootfsRequest is the request body for forking a rootfs
type forkRootfsRequest struct {
	NewSandboxID string `json:"new_sandbox_id"`
}

// getRootfs handles GET /sandboxes/{sandbox_id}/rootfs
func (s *Server) getRootfs(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		_ = spec.WriteError(w, http.StatusUnauthorized, spec.CodeUnauthorized, "unauthorized")
		return
	}

	sandboxID := r.PathValue("sandbox_id")
	if sandboxID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	if s.repo == nil {
		_ = spec.WriteError(w, http.StatusServiceUnavailable, spec.CodeUnavailable, "storage service not available")
		return
	}

	rootfsInfo, err := s.repo.GetSandboxRootfs(r.Context(), sandboxID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = spec.WriteError(w, http.StatusNotFound, spec.CodeNotFound, "rootfs not found")
			return
		}
		s.logger.WithError(err).Error("Failed to get rootfs")
		_ = spec.WriteError(w, http.StatusInternalServerError, spec.CodeInternal, "internal server error")
		return
	}

	// Check if overlay is mounted (simplified check)
	mounted := s.overlayMgr != nil
	var mountPath string
	if mounted {
		if overlayCtx, err := s.overlayMgr.GetOverlay(sandboxID); err == nil {
			mounted = overlayCtx.Mounted
		} else {
			mounted = false
		}
	}

	resp := rootfsInfoResponse{
		SandboxID:     rootfsInfo.SandboxID,
		TeamID:        rootfsInfo.TeamID,
		BaseLayerID:   rootfsInfo.BaseLayerID,
		UpperVolumeID: rootfsInfo.UpperVolumeID,
		UpperPath:     rootfsInfo.UpperPath,
		WorkPath:      rootfsInfo.WorkPath,
		MountPath:     mountPath,
		Mounted:       mounted,
		CreatedAt:     rootfsInfo.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     rootfsInfo.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	_ = spec.WriteSuccess(w, http.StatusOK, resp)
}

// createRootfsSnapshot handles POST /sandboxes/{sandbox_id}/rootfs/snapshots
func (s *Server) createRootfsSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		_ = spec.WriteError(w, http.StatusUnauthorized, spec.CodeUnauthorized, "unauthorized")
		return
	}

	sandboxID := r.PathValue("sandbox_id")
	if sandboxID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	var req createRootfsSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "name is required")
		return
	}

	if s.rootfsSnapshotSvc == nil {
		_ = spec.WriteError(w, http.StatusServiceUnavailable, spec.CodeUnavailable, "rootfs snapshot service not available")
		return
	}

	snap, err := s.rootfsSnapshotSvc.CreateSnapshot(r.Context(), &rootfs.CreateSnapshotRequest{
		SandboxID:        sandboxID,
		TeamID:           claims.TeamID,
		Name:             req.Name,
		Description:      req.Description,
		Metadata:         req.Metadata,
		RetentionSeconds: req.RetentionSeconds,
	})
	if err != nil {
		s.handleRootfsError(w, err)
		return
	}

	resp := s.rootfsSnapshotToResponse(snap)
	_ = spec.WriteSuccess(w, http.StatusCreated, resp)
}

// listRootfsSnapshots handles GET /sandboxes/{sandbox_id}/rootfs/snapshots
func (s *Server) listRootfsSnapshots(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		_ = spec.WriteError(w, http.StatusUnauthorized, spec.CodeUnauthorized, "unauthorized")
		return
	}

	sandboxID := r.PathValue("sandbox_id")
	if sandboxID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	// Parse pagination params
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	if s.rootfsSnapshotSvc == nil {
		_ = spec.WriteError(w, http.StatusServiceUnavailable, spec.CodeUnavailable, "rootfs snapshot service not available")
		return
	}

	snapshots, total, err := s.rootfsSnapshotSvc.ListSnapshots(r.Context(), sandboxID, limit, offset)
	if err != nil {
		s.handleRootfsError(w, err)
		return
	}

	var responses []rootfsSnapshotResponse
	for _, snap := range snapshots {
		responses = append(responses, s.rootfsSnapshotToResponse(snap))
	}

	_ = spec.WriteSuccess(w, http.StatusOK, map[string]any{
		"items": responses,
		"total": total,
	})
}

// getRootfsSnapshot handles GET /sandboxes/{sandbox_id}/rootfs/snapshots/{snapshot_id}
func (s *Server) getRootfsSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		_ = spec.WriteError(w, http.StatusUnauthorized, spec.CodeUnauthorized, "unauthorized")
		return
	}

	sandboxID := r.PathValue("sandbox_id")
	snapshotID := r.PathValue("snapshot_id")
	if sandboxID == "" || snapshotID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and snapshot_id are required")
		return
	}

	if s.rootfsSnapshotSvc == nil {
		_ = spec.WriteError(w, http.StatusServiceUnavailable, spec.CodeUnavailable, "rootfs snapshot service not available")
		return
	}

	snap, err := s.rootfsSnapshotSvc.GetSnapshot(r.Context(), snapshotID)
	if err != nil {
		s.handleRootfsError(w, err)
		return
	}

	// Verify snapshot belongs to the sandbox
	if snap.SandboxID != sandboxID {
		_ = spec.WriteError(w, http.StatusNotFound, spec.CodeNotFound, "snapshot not found")
		return
	}

	resp := s.rootfsSnapshotToResponse(snap)
	_ = spec.WriteSuccess(w, http.StatusOK, resp)
}

// restoreRootfsSnapshot handles POST /sandboxes/{sandbox_id}/rootfs/snapshots/{snapshot_id}/restore
func (s *Server) restoreRootfsSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		_ = spec.WriteError(w, http.StatusUnauthorized, spec.CodeUnauthorized, "unauthorized")
		return
	}

	sandboxID := r.PathValue("sandbox_id")
	snapshotID := r.PathValue("snapshot_id")
	if sandboxID == "" || snapshotID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and snapshot_id are required")
		return
	}

	var req restoreRootfsSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Request body is optional for restore
		req = restoreRootfsSnapshotRequest{}
	}

	if s.rootfsSnapshotSvc == nil {
		_ = spec.WriteError(w, http.StatusServiceUnavailable, spec.CodeUnavailable, "rootfs snapshot service not available")
		return
	}

	backupID, err := s.rootfsSnapshotSvc.RestoreSnapshot(r.Context(), &rootfs.RestoreSnapshotRequest{
		SandboxID:    sandboxID,
		TeamID:       claims.TeamID,
		SnapshotID:   snapshotID,
		CreateBackup: req.CreateBackup,
		BackupName:   req.BackupName,
	})
	if err != nil {
		s.handleRootfsError(w, err)
		return
	}

	resp := map[string]any{
		"success": true,
	}
	if backupID != "" {
		resp["backup_snapshot_id"] = backupID
	}

	_ = spec.WriteSuccess(w, http.StatusOK, resp)
}

// deleteRootfsSnapshot handles DELETE /sandboxes/{sandbox_id}/rootfs/snapshots/{snapshot_id}
func (s *Server) deleteRootfsSnapshot(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		_ = spec.WriteError(w, http.StatusUnauthorized, spec.CodeUnauthorized, "unauthorized")
		return
	}

	sandboxID := r.PathValue("sandbox_id")
	snapshotID := r.PathValue("snapshot_id")
	if sandboxID == "" || snapshotID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and snapshot_id are required")
		return
	}

	if s.rootfsSnapshotSvc == nil {
		_ = spec.WriteError(w, http.StatusServiceUnavailable, spec.CodeUnavailable, "rootfs snapshot service not available")
		return
	}

	err := s.rootfsSnapshotSvc.DeleteSnapshot(r.Context(), sandboxID, snapshotID)
	if err != nil {
		if errors.Is(err, rootfs.ErrSnapshotNotFound) || errors.Is(err, rootfs.ErrInvalidSnapshotID) {
			_ = spec.WriteSuccess(w, http.StatusOK, map[string]bool{"deleted": true})
			return
		}
		s.handleRootfsError(w, err)
		return
	}

	_ = spec.WriteSuccess(w, http.StatusOK, map[string]bool{"deleted": true})
}

// forkRootfs handles POST /sandboxes/{sandbox_id}/rootfs/fork
func (s *Server) forkRootfs(w http.ResponseWriter, r *http.Request) {
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		_ = spec.WriteError(w, http.StatusUnauthorized, spec.CodeUnauthorized, "unauthorized")
		return
	}

	sandboxID := r.PathValue("sandbox_id")
	if sandboxID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	var req forkRootfsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, err.Error())
		return
	}

	if req.NewSandboxID == "" {
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "new_sandbox_id is required")
		return
	}

	if s.rootfsSnapshotSvc == nil {
		_ = spec.WriteError(w, http.StatusServiceUnavailable, spec.CodeUnavailable, "rootfs snapshot service not available")
		return
	}

	targetRootfs, err := s.rootfsSnapshotSvc.Fork(r.Context(), &rootfs.ForkRequest{
		SourceSandboxID: sandboxID,
		TargetSandboxID: req.NewSandboxID,
		TeamID:          claims.TeamID,
	})
	if err != nil {
		s.handleRootfsError(w, err)
		return
	}

	resp := rootfsInfoResponse{
		SandboxID:     targetRootfs.SandboxID,
		TeamID:        targetRootfs.TeamID,
		BaseLayerID:   targetRootfs.BaseLayerID,
		UpperVolumeID: targetRootfs.UpperVolumeID,
		UpperPath:     targetRootfs.UpperPath,
		WorkPath:      targetRootfs.WorkPath,
		CreatedAt:     targetRootfs.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     targetRootfs.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	_ = spec.WriteSuccess(w, http.StatusCreated, resp)
}

// handleRootfsError maps rootfs errors to HTTP responses
func (s *Server) handleRootfsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rootfs.ErrRootfsNotFound):
		_ = spec.WriteError(w, http.StatusNotFound, spec.CodeNotFound, "rootfs not found")
	case errors.Is(err, rootfs.ErrSnapshotNotFound):
		_ = spec.WriteError(w, http.StatusNotFound, spec.CodeNotFound, "snapshot not found")
	case errors.Is(err, rootfs.ErrSnapshotExpired):
		_ = spec.WriteError(w, http.StatusBadRequest, spec.CodeBadRequest, "snapshot has expired")
	case errors.Is(err, rootfs.ErrInvalidSnapshotID):
		_ = spec.WriteError(w, http.StatusNotFound, spec.CodeNotFound, "snapshot not found")
	default:
		s.logger.WithError(err).Error("Rootfs operation failed")
		_ = spec.WriteError(w, http.StatusInternalServerError, spec.CodeInternal, "internal server error")
	}
}

// rootfsSnapshotToResponse converts a db.RootfsSnapshot to response format
func (s *Server) rootfsSnapshotToResponse(snap *db.RootfsSnapshot) rootfsSnapshotResponse {
	resp := rootfsSnapshotResponse{
		ID:            snap.ID,
		SandboxID:     snap.SandboxID,
		TeamID:        snap.TeamID,
		BaseLayerID:   snap.BaseLayerID,
		UpperVolumeID: snap.UpperVolumeID,
		Name:          snap.Name,
		Description:   snap.Description,
		SizeBytes:     snap.SizeBytes,
		RootInode:     snap.RootInode,
		SourceInode:   snap.SourceInode,
		CreatedAt:     snap.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if snap.ExpiresAt != nil {
		expires := snap.ExpiresAt.Format("2006-01-02T15:04:05Z")
		resp.ExpiresAt = &expires
	}

	if snap.Metadata != nil {
		var metadata map[string]string
		if err := json.Unmarshal(*snap.Metadata, &metadata); err == nil {
			resp.Metadata = metadata
		}
	}

	return resp
}
