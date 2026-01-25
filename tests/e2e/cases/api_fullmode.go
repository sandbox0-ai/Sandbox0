package cases

import (
	"github.com/sandbox0-ai/infra/tests/e2e/framework"

	. "github.com/onsi/ginkgo/v2"
)

func registerApiFullModeSuite(env *framework.ScenarioEnv) {
	Describe("API full mode", func() {
		Context("key SLOs", func() {
			It("meets cold start and restore latency targets", func() {
				Skip("TODO: implement latency assertions for full mode")
			})
		})
	})
}
