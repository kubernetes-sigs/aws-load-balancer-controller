package globalaccelerator

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var tf *framework.Framework

func TestGlobalAccelerator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GlobalAccelerator Suite")
}

var _ = BeforeSuite(func() {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())

	if !utils.IsCommercialPartition(tf.Options.AWSRegion) {
		Skip("GlobalAccelerator is only available in commercial AWS partition")
	}
})
