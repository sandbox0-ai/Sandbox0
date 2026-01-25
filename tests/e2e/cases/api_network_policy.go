package cases

import (
	"github.com/sandbox0-ai/infra/tests/e2e/framework"

	. "github.com/onsi/ginkgo/v2"
)

func registerApiNetworkPolicySuite(env *framework.ScenarioEnv) {
	Describe("API network policy mode", func() {
		Context("concurrency and isolation", func() {
			It("prevents conflicts across concurrent users", func() {
				Skip("TODO: implement network policy isolation checks via API")
			})
		})
	})
}
