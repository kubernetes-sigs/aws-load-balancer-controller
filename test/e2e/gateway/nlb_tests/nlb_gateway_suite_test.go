package nlb_tests

import (
	"testing"

	"sigs.k8s.io/aws-load-balancer-controller/test/framework"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var tf *framework.Framework

func TestNLBGateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NLB Gateway Suite")
}

var _ = BeforeSuite(func() {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())
})
