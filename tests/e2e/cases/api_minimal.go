package cases

import "github.com/sandbox0-ai/sandbox0/pkg/framework"

func registerApiMinimalSuite(envProvider func() *framework.ScenarioEnv) {
	registerApiModeSuite(envProvider, apiModeSuiteOptions{
		name:                     "minimal",
		describe:                 "API minimal mode",
		templateNamePrefix:       "e2e-minimal",
		fileContent:              "hello minimal",
		includeSandboxListTests:  true,
		expectStorageUnavailable: true,
		expectNetworkUnavailable: true,
	})
}
