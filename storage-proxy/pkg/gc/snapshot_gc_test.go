package gc

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 5*time.Minute, cfg.Interval)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.True(t, cfg.Enabled)
}

func TestSnapshotGC_ConfigDefaults(t *testing.T) {
	logger := logrus.New()

	// Test with zero values - should use defaults
	gc := NewSnapshotGC(nil, nil, nil, nil, Config{}, logger)
	assert.Equal(t, 5*time.Minute, gc.config.Interval)
	assert.Equal(t, 100, gc.config.BatchSize)

	// Test with custom values
	gc = NewSnapshotGC(nil, nil, nil, nil, Config{
		Interval:  10 * time.Minute,
		BatchSize: 50,
	}, logger)
	assert.Equal(t, 10*time.Minute, gc.config.Interval)
	assert.Equal(t, 50, gc.config.BatchSize)
}

func TestSnapshotGC_Disabled(t *testing.T) {
	logger := logrus.New()
	gc := NewSnapshotGC(nil, nil, nil, nil, Config{Enabled: false}, logger)

	err := gc.Start(context.Background())
	require.NoError(t, err)

	// Should return immediately when disabled
	gc.Stop()
}

func TestSnapshotGC_StartStop(t *testing.T) {
	logger := logrus.New()
	gc := NewSnapshotGC(nil, nil, nil, nil, Config{
		Enabled:  true,
		Interval: 1 * time.Second,
	}, logger)

	// Start should succeed
	err := gc.Start(context.Background())
	require.NoError(t, err)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop should work
	gc.Stop()
}

func TestSnapshotGC_RunOnce_NilServices(t *testing.T) {
	logger := logrus.New()
	// When services are nil, RunOnce should handle gracefully
	gc := NewSnapshotGC(nil, nil, nil, nil, Config{}, logger)

	// This should not panic even with nil services
	// It will fail to list expired snapshots but should not crash
	gc.runOnce(context.Background())
}
