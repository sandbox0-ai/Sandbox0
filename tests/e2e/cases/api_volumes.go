package cases

import (
	"github.com/sandbox0-ai/infra/tests/e2e/framework"

	. "github.com/onsi/ginkgo/v2"
)

func registerApiVolumesSuite(env *framework.ScenarioEnv) {
	Describe("API volumes mode", func() {
		Context("snapshot and restore", func() {
			It("restores from snapshot with consistent data", func() {
				Skip("TODO: implement snapshot restore via API in volumes mode")
			})
		})

		Context("filesystem and process capabilities", func() {
			It("performs file operations and command execution", func() {
				Skip("TODO: implement filesystem and process APIs for volumes mode")
			})
		})
	})
}
