package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/pkg/gateway/spec"
)

// === Rootfs Management Handlers (→ Storage Proxy) ===

// getRootfs gets rootfs info for a sandbox
func (s *Server) getRootfs(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "id is required")
		return
	}

	c.Request.URL.Path = "/sandboxes/" + id + "/rootfs"
	s.proxyToStorageProxy(c)
}

// createRootfsSnapshot creates a rootfs snapshot
func (s *Server) createRootfsSnapshot(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "id is required")
		return
	}

	c.Request.URL.Path = "/sandboxes/" + id + "/rootfs/snapshots"
	s.proxyToStorageProxy(c)
}

// listRootfsSnapshots lists rootfs snapshots for a sandbox
func (s *Server) listRootfsSnapshots(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "id is required")
		return
	}

	c.Request.URL.Path = "/sandboxes/" + id + "/rootfs/snapshots"
	// Forward query parameters for pagination
	s.proxyToStorageProxy(c)
}

// getRootfsSnapshot gets a specific rootfs snapshot
func (s *Server) getRootfsSnapshot(c *gin.Context) {
	id := c.Param("id")
	snapshotID := c.Param("snapshot_id")
	if id == "" || snapshotID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "id and snapshot_id are required")
		return
	}

	c.Request.URL.Path = "/sandboxes/" + id + "/rootfs/snapshots/" + snapshotID
	s.proxyToStorageProxy(c)
}

// restoreRootfsSnapshot restores a rootfs from a snapshot
func (s *Server) restoreRootfsSnapshot(c *gin.Context) {
	id := c.Param("id")
	snapshotID := c.Param("snapshot_id")
	if id == "" || snapshotID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "id and snapshot_id are required")
		return
	}

	c.Request.URL.Path = "/sandboxes/" + id + "/rootfs/snapshots/" + snapshotID + "/restore"
	s.proxyToStorageProxy(c)
}

// deleteRootfsSnapshot deletes a rootfs snapshot
func (s *Server) deleteRootfsSnapshot(c *gin.Context) {
	id := c.Param("id")
	snapshotID := c.Param("snapshot_id")
	if id == "" || snapshotID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "id and snapshot_id are required")
		return
	}

	c.Request.URL.Path = "/sandboxes/" + id + "/rootfs/snapshots/" + snapshotID
	s.proxyToStorageProxy(c)
}

// forkRootfs forks a sandbox rootfs
func (s *Server) forkRootfs(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "id is required")
		return
	}

	c.Request.URL.Path = "/sandboxes/" + id + "/rootfs/fork"
	s.proxyToStorageProxy(c)
}
