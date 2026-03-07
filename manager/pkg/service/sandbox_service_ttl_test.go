package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sandbox0-ai/sandbox0/manager/pkg/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time                  { return c.now }
func (c fixedClock) Since(t time.Time) time.Duration { return c.now.Sub(t) }
func (c fixedClock) Until(t time.Time) time.Duration { return t.Sub(c.now) }

func newSandboxServiceForTTLTests(t *testing.T, pod *corev1.Pod, defaultTTL time.Duration) (*SandboxService, *fake.Clientset) {
	t.Helper()

	client := fake.NewSimpleClientset(pod.DeepCopy())
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	require.NoError(t, indexer.Add(pod.DeepCopy()))

	return &SandboxService{
		k8sClient: client,
		podLister: corelisters.NewPodLister(indexer),
		clock: fixedClock{
			now: time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC),
		},
		config: SandboxServiceConfig{
			DefaultTTL: defaultTTL,
		},
		logger: zap.NewNop(),
	}, client
}

func testSandboxPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-1",
			Namespace: "default",
			Labels: map[string]string{
				controller.LabelSandboxID: "sandbox-1",
			},
			Annotations: map[string]string{
				controller.AnnotationTeamID: "team-1",
				controller.AnnotationUserID: "user-1",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.10",
		},
	}
}

func TestClaimConfigForPersistence(t *testing.T) {
	t.Run("omitted ttl uses default", func(t *testing.T) {
		svc := &SandboxService{config: SandboxServiceConfig{DefaultTTL: 5 * time.Minute}}
		cfg := svc.claimConfigForPersistence(nil)
		require.NotNil(t, cfg)
		require.NotNil(t, cfg.TTL)
		assert.Equal(t, int32(300), *cfg.TTL)
		assert.Nil(t, cfg.HardTTL)
	})

	t.Run("explicit zero remains disabled", func(t *testing.T) {
		svc := &SandboxService{config: SandboxServiceConfig{DefaultTTL: 5 * time.Minute}}
		cfg := svc.claimConfigForPersistence(&SandboxConfig{
			TTL:     int32Ptr(0),
			HardTTL: int32Ptr(0),
		})
		require.NotNil(t, cfg)
		require.NotNil(t, cfg.TTL)
		require.NotNil(t, cfg.HardTTL)
		assert.Equal(t, int32(0), *cfg.TTL)
		assert.Equal(t, int32(0), *cfg.HardTTL)
	})

	t.Run("zero default keeps ttl disabled when omitted", func(t *testing.T) {
		svc := &SandboxService{config: SandboxServiceConfig{DefaultTTL: 0}}
		assert.Nil(t, svc.claimConfigForPersistence(nil))
	})
}

func TestUpdateSandboxZeroTTLDisablesExpirations(t *testing.T) {
	pod := testSandboxPod()
	pod.Annotations[controller.AnnotationExpiresAt] = "2026-03-07T12:05:00Z"
	pod.Annotations[controller.AnnotationHardExpiresAt] = "2026-03-07T12:10:00Z"
	pod.Annotations[controller.AnnotationConfig] = `{"ttl":300,"hard_ttl":600}`

	svc, client := newSandboxServiceForTTLTests(t, pod, 0)

	updated, err := svc.UpdateSandbox(context.Background(), pod.Name, &SandboxUpdateConfig{
		TTL:     int32Ptr(0),
		HardTTL: int32Ptr(0),
	})
	require.NoError(t, err)
	assert.True(t, updated.ExpiresAt.IsZero())
	assert.True(t, updated.HardExpiresAt.IsZero())

	stored, err := client.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, stored.Annotations[controller.AnnotationExpiresAt])
	assert.Empty(t, stored.Annotations[controller.AnnotationHardExpiresAt])

	var cfg SandboxConfig
	require.NoError(t, json.Unmarshal([]byte(stored.Annotations[controller.AnnotationConfig]), &cfg))
	require.NotNil(t, cfg.TTL)
	require.NotNil(t, cfg.HardTTL)
	assert.Equal(t, int32(0), *cfg.TTL)
	assert.Equal(t, int32(0), *cfg.HardTTL)
}

func TestRefreshSandboxDisabledTTLRemainsDisabled(t *testing.T) {
	pod := testSandboxPod()
	pod.Annotations[controller.AnnotationConfig] = `{"ttl":0,"hard_ttl":0}`

	svc, client := newSandboxServiceForTTLTests(t, pod, 0)

	resp, err := svc.RefreshSandbox(context.Background(), pod.Name, &RefreshRequest{})
	require.NoError(t, err)
	assert.True(t, resp.ExpiresAt.IsZero())
	assert.True(t, resp.HardExpiresAt.IsZero())

	stored, err := client.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, stored.Annotations[controller.AnnotationExpiresAt])
	assert.Empty(t, stored.Annotations[controller.AnnotationHardExpiresAt])
}
