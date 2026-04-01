package chained_gateway_tests

import (
	"testing"

	"sigs.k8s.io/aws-load-balancer-controller/test/framework"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var tf *framework.Framework

func TestChainedGateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Chained Gateway Suite")
}

var _ = BeforeSuite(func() {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())
})
