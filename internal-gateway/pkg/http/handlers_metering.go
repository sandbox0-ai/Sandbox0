package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/sandbox0/pkg/gateway/spec"
	"go.uber.org/zap"
)

func (s *Server) getMeteringStatus(c *gin.Context) {
	if s.meteringRepo == nil {
		spec.JSONError(c, http.StatusServiceUnavailable, spec.CodeUnavailable, "metering is unavailable")
		return
	}

	status, err := s.meteringRepo.GetStatus(c.Request.Context(), s.cfg.RegionID)
	if err != nil {
		s.logger.Error("Failed to load metering status", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "failed to load metering status")
		return
	}

	spec.JSONSuccess(c, http.StatusOK, status)
}

func (s *Server) listMeteringEvents(c *gin.Context) {
	if s.meteringRepo == nil {
		spec.JSONError(c, http.StatusServiceUnavailable, spec.CodeUnavailable, "metering is unavailable")
		return
	}

	afterSequence := int64(0)
	if value := c.Query("after_sequence"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "invalid after_sequence")
			return
		}
		afterSequence = parsed
	}

	limit := 100
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "invalid limit")
			return
		}
		if parsed > 1000 {
			parsed = 1000
		}
		limit = parsed
	}

	events, err := s.meteringRepo.ListEventsAfter(c.Request.Context(), afterSequence, limit)
	if err != nil {
		s.logger.Error("Failed to list metering events", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "failed to list metering events")
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"events": events,
	})
}

func (s *Server) listMeteringWindows(c *gin.Context) {
	if s.meteringRepo == nil {
		spec.JSONError(c, http.StatusServiceUnavailable, spec.CodeUnavailable, "metering is unavailable")
		return
	}

	afterSequence := int64(0)
	if value := c.Query("after_sequence"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "invalid after_sequence")
			return
		}
		afterSequence = parsed
	}

	limit := 100
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "invalid limit")
			return
		}
		if parsed > 1000 {
			parsed = 1000
		}
		limit = parsed
	}

	windows, err := s.meteringRepo.ListWindowsAfter(c.Request.Context(), afterSequence, limit)
	if err != nil {
		s.logger.Error("Failed to list metering windows", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "failed to list metering windows")
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"windows": windows,
	})
}
