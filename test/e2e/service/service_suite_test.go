package service_test

import (
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var tf *framework.Framework

func TestService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service Suite")
}

var _ = BeforeSuite(func() {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())
})
