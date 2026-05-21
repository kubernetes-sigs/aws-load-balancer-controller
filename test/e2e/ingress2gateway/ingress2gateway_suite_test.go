package ingress2gateway

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
)

var tf *framework.Framework

func TestIngress2Gateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingress2Gateway Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())

	if tf.Options.ControllerImage != "" {
		err = tf.CTRLInstallationManager.UpgradeController(tf.Options.ControllerImage, true, true, false)
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(60 * time.Second)
	}
	return nil
}, func(data []byte) {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())
})
