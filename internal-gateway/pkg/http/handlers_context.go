package http

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/internal-gateway/pkg/middleware"
	"github.com/sandbox0-ai/infra/pkg/gateway/spec"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/pkg/proxy"
	"go.uber.org/zap"
)

// === Process/Context Management Handlers (→ Procd) ===

// createContext creates a new context in a sandbox
func (s *Server) createContext(c *gin.Context) {
	sandboxID := c.Param("id")
	if sandboxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return // Error response already sent
	}

	// Rewrite path for procd
	c.Request.URL.Path = "/api/v1/contexts"

	s.proxyToProcd(c, procdURL)
}

// listContexts lists all contexts in a sandbox
func (s *Server) listContexts(c *gin.Context) {
	sandboxID := c.Param("id")
	if sandboxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts"
	s.proxyToProcd(c, procdURL)
}

// getContext gets a specific context
func (s *Server) getContext(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts/" + ctxID
	s.proxyToProcd(c, procdURL)
}

// deleteContext deletes a context
func (s *Server) deleteContext(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts/" + ctxID
	s.proxyToProcd(c, procdURL)
}

// restartContext restarts a context
func (s *Server) restartContext(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts/" + ctxID + "/restart"
	s.proxyToProcd(c, procdURL)
}

// contextInput sends input to a context
func (s *Server) contextInput(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts/" + ctxID + "/input"
	s.proxyToProcd(c, procdURL)
}

// contextResize resizes a context
func (s *Server) contextResize(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts/" + ctxID + "/resize"
	s.proxyToProcd(c, procdURL)
}

// contextSignal sends a signal to a context
func (s *Server) contextSignal(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts/" + ctxID + "/signal"
	s.proxyToProcd(c, procdURL)
}

// contextStats gets stats for a context
func (s *Server) contextStats(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	c.Request.URL.Path = "/api/v1/contexts/" + ctxID + "/stats"
	s.proxyToProcd(c, procdURL)
}

// contextWebSocket handles WebSocket connections for context
func (s *Server) contextWebSocket(c *gin.Context) {
	sandboxID := c.Param("id")
	ctxID := c.Param("ctx_id")
	if sandboxID == "" || ctxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and ctx_id are required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	requestModifier, err := s.buildProcdRequestModifier(c)
	if err != nil {
		return
	}

	// Handle WebSocket upgrade
	wsProxy := proxy.NewWebSocketProxy(s.logger, proxy.WithRequestModifier(requestModifier))
	c.Request.URL.Path = "/api/v1/contexts/" + ctxID + "/ws"
	wsProxy.Proxy(procdURL)(c)
}

// getProcdURL resolves the procd URL for a sandbox
// Uses in-memory cache to reduce manager API calls and improve performance
func (s *Server) getProcdURL(c *gin.Context, sandboxID string) (*url.URL, error) {
	authCtx := middleware.GetAuthContext(c)

	// Try to get from cache first
	var addr *url.URL
	if cached, ok := s.sandboxAddrCache.Get(sandboxID); ok {
		addr = cached
		s.logger.Debug("Sandbox cache hit",
			zap.String("sandbox_id", sandboxID),
		)
	} else {
		// Cache miss - fetch from manager
		s.logger.Debug("Sandbox cache miss, fetching from manager",
			zap.String("sandbox_id", sandboxID),
		)

		sandbox, err := s.managerClient.GetSandbox(c.Request.Context(), sandboxID, authCtx.UserID, authCtx.TeamID)
		if err != nil {
			s.logger.Error("Failed to get sandbox from manager",
				zap.String("sandbox_id", sandboxID),
				zap.Error(err),
			)
			spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "sandbox not found")
			return nil, err
		}

		// Verify team ownership
		if sandbox.TeamID != authCtx.TeamID {
			spec.JSONError(c, http.StatusForbidden, spec.CodeForbidden, "sandbox belongs to a different team")
			return nil, errors.New("sandbox belongs to a different team")
		}

		// Parse procd address
		addr, err = url.Parse(sandbox.InternalAddr)
		if err != nil {
			s.logger.Error("Invalid procd address",
				zap.String("sandbox_id", sandboxID),
				zap.String("procd_address", sandbox.InternalAddr),
				zap.Error(err),
			)
			spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "invalid procd address")
			return nil, err
		}

		// Store in cache for future requests
		s.sandboxAddrCache.Set(sandboxID, addr)
		s.logger.Debug("Sandbox cached",
			zap.String("sandbox_id", sandboxID),
			zap.String("internal_addr", addr.String()),
		)
	}

	return addr, nil
}

func (s *Server) buildProcdRequestModifier(c *gin.Context) (proxy.RequestModifier, error) {
	authCtx := middleware.GetAuthContext(c)

	// Generate internal token for procd
	internalToken, err := s.internalAuthGen.Generate("procd", authCtx.TeamID, authCtx.UserID, internalauth.GenerateOptions{})
	if err != nil {
		s.logger.Error("Failed to generate internal token for procd",
			zap.String("team_id", authCtx.TeamID),
			zap.Error(err),
		)
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "internal authentication failed")
		return nil, err
	}

	// Generate a special token for procd to communicate with storage-proxy
	// This token allows procd to access storage-proxy on behalf of this team
	perms := s.cfg.ProcdStoragePermissions
	if len(perms) == 0 {
		perms = []string{"sandboxvolume:read", "sandboxvolume:write"}
	}
	procdStorageToken, err := s.procdAuthGen.Generate("storage-proxy", authCtx.TeamID, authCtx.UserID, internalauth.GenerateOptions{
		Permissions: perms,
	})
	if err != nil {
		s.logger.Error("Failed to generate procd-storage token",
			zap.String("team_id", authCtx.TeamID),
			zap.Error(err),
		)
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "internal authentication failed")
		return nil, err
	}

	return func(req *http.Request) {
		req.Header.Set(internalauth.TeamIDHeader, authCtx.TeamID)
		req.Header.Set(internalauth.DefaultTokenHeader, internalToken)
		req.Header.Set(internalauth.TokenForProcdHeader, procdStorageToken)
	}, nil
}

// proxyToProcd proxies a request to a specific procd instance
func (s *Server) proxyToProcd(c *gin.Context, procdURL *url.URL) {
	requestModifier, err := s.buildProcdRequestModifier(c)
	if err != nil {
		return
	}

	// Create and execute reverse proxy
	proxyTimeout := s.cfg.ProxyTimeout.Duration
	if proxyTimeout == 0 {
		proxyTimeout = 10 * time.Second
	}
	router, err := proxy.NewRouter(procdURL.String(), s.logger, proxyTimeout, proxy.WithRequestModifier(requestModifier))
	if err != nil {
		s.logger.Error("Failed to create procd proxy router",
			zap.String("procd_url", procdURL.String()),
			zap.Error(err),
		)
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "proxy initialization failed")
		return
	}

	router.ProxyToTarget(c)
}
