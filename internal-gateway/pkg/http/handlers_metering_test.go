package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/sandbox0/infra-operator/api/config"
	"github.com/sandbox0-ai/sandbox0/pkg/gateway/spec"
	"github.com/sandbox0-ai/sandbox0/pkg/metering"
	"go.uber.org/zap"
)

type fakeMeteringReader struct {
	status         *metering.Status
	statusErr      error
	events         []*metering.Event
	eventsErr      error
	windows        []*metering.Window
	windowsErr     error
	gotFallback    string
	gotAfter       int64
	gotLimit       int
	gotWindowAfter int64
	gotWindowLimit int
	statusCalls    int
	listEventCalls int
	listWinCalls   int
}

func (f *fakeMeteringReader) GetStatus(_ context.Context, fallbackRegionID string) (*metering.Status, error) {
	f.statusCalls++
	f.gotFallback = fallbackRegionID
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}

func (f *fakeMeteringReader) ListEventsAfter(_ context.Context, afterSequence int64, limit int) ([]*metering.Event, error) {
	f.listEventCalls++
	f.gotAfter = afterSequence
	f.gotLimit = limit
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	return f.events, nil
}

func (f *fakeMeteringReader) ListWindowsAfter(_ context.Context, afterSequence int64, limit int) ([]*metering.Window, error) {
	f.listWinCalls++
	f.gotWindowAfter = afterSequence
	f.gotWindowLimit = limit
	if f.windowsErr != nil {
		return nil, f.windowsErr
	}
	return f.windows, nil
}

type meteringEventsResponse struct {
	Events []*metering.Event `json:"events"`
}

type meteringWindowsResponse struct {
	Windows []*metering.Window `json:"windows"`
}

func TestGetMeteringStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns unavailable when metering repo is not configured", func(t *testing.T) {
		server := &Server{
			cfg:    &config.InternalGatewayConfig{},
			logger: zap.NewNop(),
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/status", nil)

		server.getMeteringStatus(ctx)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("returns metering status and forwards region fallback", func(t *testing.T) {
		completeBefore := time.Date(2026, 3, 12, 8, 0, 0, 0, time.UTC)
		repo := &fakeMeteringReader{
			status: &metering.Status{
				RegionID:             "aws/us-east-1",
				LatestEventSequence:  42,
				LatestWindowSequence: 18,
				CompleteBefore:       &completeBefore,
				ProducerCount:        2,
			},
		}
		server := &Server{
			cfg:          &config.InternalGatewayConfig{GatewayConfig: config.GatewayConfig{RegionID: "aws/us-east-1"}},
			logger:       zap.NewNop(),
			meteringRepo: repo,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/status", nil)

		server.getMeteringStatus(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if repo.gotFallback != "aws/us-east-1" {
			t.Fatalf("fallback region = %q, want %q", repo.gotFallback, "aws/us-east-1")
		}

		resp, apiErr, err := spec.DecodeResponse[metering.Status](recorder.Body)
		if err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if apiErr != nil {
			t.Fatalf("unexpected api error: %+v", apiErr)
		}
		if resp.LatestEventSequence != 42 {
			t.Fatalf("latest_event_sequence = %d, want 42", resp.LatestEventSequence)
		}
		if resp.LatestWindowSequence != 18 {
			t.Fatalf("latest_window_sequence = %d, want 18", resp.LatestWindowSequence)
		}
		if resp.ProducerCount != 2 {
			t.Fatalf("producer_count = %d, want 2", resp.ProducerCount)
		}
		if resp.CompleteBefore == nil || !resp.CompleteBefore.Equal(completeBefore) {
			t.Fatalf("complete_before = %v, want %v", resp.CompleteBefore, completeBefore)
		}
	})
}

func TestListMeteringEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("rejects invalid after_sequence", func(t *testing.T) {
		repo := &fakeMeteringReader{}
		server := &Server{
			cfg:          &config.InternalGatewayConfig{},
			logger:       zap.NewNop(),
			meteringRepo: repo,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/events?after_sequence=bad", nil)

		server.listMeteringEvents(ctx)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
		if repo.listEventCalls != 0 {
			t.Fatalf("list events should not be called for invalid after_sequence")
		}
	})

	t.Run("clamps limit and returns events", func(t *testing.T) {
		occurredAt := time.Date(2026, 3, 12, 9, 30, 0, 0, time.UTC)
		repo := &fakeMeteringReader{
			events: []*metering.Event{
				{
					Sequence:    11,
					EventID:     "volume/vol-1/created/1",
					Producer:    "storage-proxy.volume",
					RegionID:    "aws/us-east-1",
					EventType:   metering.EventTypeVolumeCreated,
					SubjectType: metering.SubjectTypeVolume,
					SubjectID:   "vol-1",
					TeamID:      "team-1",
					VolumeID:    "vol-1",
					OccurredAt:  occurredAt,
					RecordedAt:  occurredAt,
				},
			},
		}
		server := &Server{
			cfg:          &config.InternalGatewayConfig{},
			logger:       zap.NewNop(),
			meteringRepo: repo,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/events?after_sequence=10&limit=5000", nil)

		server.listMeteringEvents(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if repo.gotAfter != 10 {
			t.Fatalf("after_sequence = %d, want 10", repo.gotAfter)
		}
		if repo.gotLimit != 1000 {
			t.Fatalf("limit = %d, want 1000", repo.gotLimit)
		}

		resp, apiErr, err := spec.DecodeResponse[meteringEventsResponse](recorder.Body)
		if err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if apiErr != nil {
			t.Fatalf("unexpected api error: %+v", apiErr)
		}
		if len(resp.Events) != 1 {
			t.Fatalf("event count = %d, want 1", len(resp.Events))
		}
		if resp.Events[0].Sequence != 11 {
			t.Fatalf("sequence = %d, want 11", resp.Events[0].Sequence)
		}
	})

	t.Run("returns internal error when repo lookup fails", func(t *testing.T) {
		repo := &fakeMeteringReader{eventsErr: errors.New("boom")}
		server := &Server{
			cfg:          &config.InternalGatewayConfig{},
			logger:       zap.NewNop(),
			meteringRepo: repo,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/events", nil)

		server.listMeteringEvents(ctx)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})
}

func TestListMeteringWindows(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("rejects invalid limit", func(t *testing.T) {
		repo := &fakeMeteringReader{}
		server := &Server{
			cfg:          &config.InternalGatewayConfig{},
			logger:       zap.NewNop(),
			meteringRepo: repo,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/windows?limit=bad", nil)

		server.listMeteringWindows(ctx)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
		if repo.listWinCalls != 0 {
			t.Fatalf("list windows should not be called for invalid limit")
		}
	})

	t.Run("clamps limit and returns windows", func(t *testing.T) {
		windowStart := time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC)
		windowEnd := windowStart.Add(5 * time.Minute)
		repo := &fakeMeteringReader{
			windows: []*metering.Window{
				{
					Sequence:    3,
					WindowID:    "sandbox/sb-1/windows/sandbox.active_seconds/1/2",
					Producer:    "manager.sandbox_lifecycle",
					RegionID:    "aws/us-east-1",
					WindowType:  metering.WindowTypeSandboxActiveSeconds,
					SubjectType: metering.SubjectTypeSandbox,
					SubjectID:   "sb-1",
					TeamID:      "team-1",
					UserID:      "user-1",
					SandboxID:   "sb-1",
					TemplateID:  "tpl-1",
					ClusterID:   "cluster-a",
					WindowStart: windowStart,
					WindowEnd:   windowEnd,
					Value:       300,
					Unit:        metering.WindowUnitSeconds,
					RecordedAt:  windowEnd,
				},
			},
		}
		server := &Server{
			cfg:          &config.InternalGatewayConfig{},
			logger:       zap.NewNop(),
			meteringRepo: repo,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/windows?after_sequence=2&limit=5000", nil)

		server.listMeteringWindows(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if repo.gotWindowAfter != 2 {
			t.Fatalf("after_sequence = %d, want 2", repo.gotWindowAfter)
		}
		if repo.gotWindowLimit != 1000 {
			t.Fatalf("limit = %d, want 1000", repo.gotWindowLimit)
		}

		resp, apiErr, err := spec.DecodeResponse[meteringWindowsResponse](recorder.Body)
		if err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if apiErr != nil {
			t.Fatalf("unexpected api error: %+v", apiErr)
		}
		if len(resp.Windows) != 1 {
			t.Fatalf("window count = %d, want 1", len(resp.Windows))
		}
		if resp.Windows[0].Value != 300 {
			t.Fatalf("value = %d, want 300", resp.Windows[0].Value)
		}
	})

	t.Run("returns internal error when window lookup fails", func(t *testing.T) {
		repo := &fakeMeteringReader{windowsErr: errors.New("boom")}
		server := &Server{
			cfg:          &config.InternalGatewayConfig{},
			logger:       zap.NewNop(),
			meteringRepo: repo,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/metering/windows", nil)

		server.listMeteringWindows(ctx)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})
}
