package middleware

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sandbox0-ai/infra/internal-gateway/pkg/db"
	"go.uber.org/zap"
)

// RequestLogger provides request logging middleware
type RequestLogger struct {
	repo        *db.Repository
	logger      *zap.Logger
	enableAudit bool
}

// NewRequestLogger creates a new request logger
func NewRequestLogger(repo *db.Repository, logger *zap.Logger, enableAudit bool) *RequestLogger {
	return &RequestLogger{
		repo:        repo,
		logger:      logger,
		enableAudit: enableAudit,
	}
}

// Logger returns a gin middleware that logs requests
func (rl *RequestLogger) Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate request ID
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		// Record start time
		start := time.Now()

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get auth context
		authCtx := GetAuthContext(c)

		// Log fields
		fields := []zap.Field{
			zap.String("request_id", requestID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if authCtx != nil {
			fields = append(fields,
				zap.String("team_id", authCtx.TeamID),
				zap.String("auth_method", string(authCtx.AuthMethod)),
			)
			if authCtx.UserID != "" {
				fields = append(fields, zap.String("user_id", authCtx.UserID))
			}
			if authCtx.APIKeyID != "" {
				fields = append(fields, zap.String("api_key_id", authCtx.APIKeyID))
			}
		}

		// Log based on status code
		status := c.Writer.Status()
		if status >= 500 {
			rl.logger.Error("HTTP request", fields...)
		} else if status >= 400 {
			rl.logger.Warn("HTTP request", fields...)
		} else {
			rl.logger.Info("HTTP request", fields...)
		}

		// Write audit log if enabled
		if rl.enableAudit && authCtx != nil && rl.repo != nil {
			authLite := &AuthContextLite{
				TeamID:   authCtx.TeamID,
				UserID:   authCtx.UserID,
				APIKeyID: authCtx.APIKeyID,
			}
			go rl.writeAuditLog(c, requestID, authLite, latency)
		}
	}
}

// writeAuditLog writes an audit log entry to the database
func (rl *RequestLogger) writeAuditLog(c *gin.Context, requestID string, authCtx *AuthContextLite, latency time.Duration) {
	// Create metadata
	metadata := map[string]interface{}{
		"query_params": c.Request.URL.RawQuery,
	}

	metadataJSON, _ := json.Marshal(metadata)

	auditLog := &db.AuditLog{
		TeamID:     authCtx.TeamID,
		UserID:     authCtx.UserID,
		APIKeyID:   authCtx.APIKeyID,
		RequestID:  requestID,
		Method:     c.Request.Method,
		Path:       c.Request.URL.Path,
		StatusCode: c.Writer.Status(),
		LatencyMs:  int(latency.Milliseconds()),
		UserAgent:  c.Request.UserAgent(),
		ClientIP:   c.ClientIP(),
		Metadata:   metadataJSON,
	}

	if err := rl.repo.CreateAuditLog(c.Request.Context(), auditLog); err != nil {
		rl.logger.Error("Failed to write audit log",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
	}
}

// AuthContextLite is a lightweight version of AuthContext for logging
type AuthContextLite struct {
	TeamID   string
	UserID   string
	APIKeyID string
}

// GetRequestID extracts request ID from gin context
func GetRequestID(c *gin.Context) string {
	if v, exists := c.Get("request_id"); exists {
		return v.(string)
	}
	return ""
}
