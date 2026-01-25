package cases

import (
	"github.com/sandbox0-ai/infra/tests/e2e/framework"

	. "github.com/onsi/ginkgo/v2"
)

func registerApiMinimalSuite(env *framework.ScenarioEnv) {
	Describe("API minimal mode", func() {
		Context("template lifecycle", func() {
			It("creates, updates, and deletes templates", func() {
				Skip("TODO: implement minimal template lifecycle via API")
			})
		})

		Context("sandbox lifecycle", func() {
			It("claims, releases, and destroys sandboxes", func() {
				Skip("TODO: implement minimal sandbox lifecycle via API")
			})
		})
	})
}
