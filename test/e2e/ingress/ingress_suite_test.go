package ingress

import (
	"fmt"
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

var _ = BeforeSuite(func() {

	fmt.Println("before!")
	var err error

	defer func() {
		fmt.Println("Hello!?")
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()

	tf, err = framework.InitFramework()

	fmt.Println("after!")

	time.Sleep(10 * time.Second)

	Expect(err).NotTo(HaveOccurred())
})
