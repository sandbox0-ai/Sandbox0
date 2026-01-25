package cases

import (
	"strings"

	"github.com/sandbox0-ai/infra/tests/e2e/framework"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// RegisterApiSuite defines API coverage for a scenario.
func RegisterApiSuite(envProvider func() *framework.ScenarioEnv) {
	Describe("API entrypoint", func() {

		env := envProvider()
		Expect(env).NotTo(BeNil())

		switch strings.ToLower(strings.TrimSpace(env.Infra.Name)) {
		case "minimal":
			registerApiMinimalSuite(env)
		case "network-policy":
			registerApiNetworkPolicySuite(env)
		case "volumes":
			registerApiVolumesSuite(env)
		case "fullmode":
			registerApiFullModeSuite(env)
		default:
			registerApiUnknownSuite(env.Infra.Name)
		}
	})
}

func registerApiUnknownSuite(infraName string) {
	Describe("API entrypoint for unknown scenario", func() {
		It("skips until scenario-specific tests exist", func() {
			Skip("no API suite registered for Sandbox0Infra name: " + infraName)
		})
	})
}
