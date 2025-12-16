package ingress

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	framework "sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"testing"
	"time"
)

var tf *framework.Framework

func TestIngress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingress Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())

	if tf.Options.ControllerImage != "" {
		err = tf.CTRLInstallationManager.UpgradeController(tf.Options.ControllerImage, true, true)
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(60 * time.Second)
	}
	return nil
}, func(data []byte) {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())
})
