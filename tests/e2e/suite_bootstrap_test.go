package e2e

import (
	"time"

	"github.com/sandbox0-ai/infra/tests/e2e/framework"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg framework.Config
var testCtx *framework.TestContext

var _ = BeforeSuite(func() {
	var err error

	cfg, err = framework.LoadConfig()
	Expect(err).NotTo(HaveOccurred())

	cluster := framework.NewCluster(cfg.ClusterName)
	testCtx = framework.NewTestContext(cluster)

	if !cfg.UseExistingCluster {
		err = cluster.CreateKind(testCtx.Context, cfg.KindConfigPath)
		Expect(err).NotTo(HaveOccurred())
	}

	if !cfg.SkipOperatorInstall {
		err = framework.InstallOperator(testCtx.Context, cfg)
		Expect(err).NotTo(HaveOccurred())

		err = framework.WaitForOperatorReady(testCtx.Context, cfg, "5m")
		Expect(err).NotTo(HaveOccurred())
	}
})

var _ = AfterSuite(func() {
	if cfg.SkipOperatorUninstall {
		return
	}

	if !cfg.SkipOperatorInstall {
		_ = framework.UninstallOperator(testCtx.Context, cfg)
		time.Sleep(2 * time.Second)
	}

	if !cfg.SkipClusterDelete && !cfg.UseExistingCluster {
		_ = testCtx.Cluster.DeleteKind(testCtx.Context)
	}
})
