package manager

import (
	"testing"

	infrav1alpha1 "github.com/sandbox0-ai/sandbox0/infra-operator/api/v1alpha1"
)

func TestResolveNetworkPolicyProvider(t *testing.T) {
	t.Run("defaults to noop when netd is disabled", func(t *testing.T) {
		infra := &infrav1alpha1.Sandbox0Infra{}
		if got := resolveNetworkPolicyProvider(infra); got != "noop" {
			t.Fatalf("expected noop provider, got %q", got)
		}
	})

	t.Run("uses netd when netd is enabled", func(t *testing.T) {
		infra := &infrav1alpha1.Sandbox0Infra{
			Spec: infrav1alpha1.Sandbox0InfraSpec{
				Services: &infrav1alpha1.ServicesConfig{
					Netd: &infrav1alpha1.NetdServiceConfig{
						BaseServiceConfig: infrav1alpha1.BaseServiceConfig{
							Enabled: true,
						},
					},
				},
			},
		}
		if got := resolveNetworkPolicyProvider(infra); got != "netd" {
			t.Fatalf("expected netd provider, got %q", got)
		}
	})
}
