package globalaccelerator

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
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

	if !isCommercialPartition(tf.Options.AWSRegion) {
		Skip("GlobalAccelerator is only available in commercial AWS partition")
	}
})

func isCommercialPartition(region string) bool {
	unsupportedPrefixes := []string{"cn-", "us-gov-", "us-iso", "eu-isoe-"}
	for _, prefix := range unsupportedPrefixes {
		if strings.HasPrefix(strings.ToLower(region), prefix) {
			return false
		}
	}
	return true
}
