package e2e_test

import (
	"flag"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"

	_ "github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/ingress"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = SynchronizedAfterSuite(func() {
	// Run on all Ginkgo nodes
	utils.Logf("Running AfterSuite actions on all nodes")
	framework.RunCleanupActions()
}, func() {
})

func TestE2E(t *testing.T) {
	flag.Parse()
	framework.ValidateGlobalOptions()

	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
