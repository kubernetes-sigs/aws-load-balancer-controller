package gateway

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
)

var _ = Describe("test k8s alb gateway using ip targets reconciled by the aws load balancer controller", func() {
	var (
		ctx     context.Context
		stack   ALBTestStack
		dnsName string
		lbARN   string
	)
	BeforeEach(func() {
		if !tf.Options.EnableGatewayTests {
			Skip("Skipping gateway tests")
		}
		ctx = context.Background()
		stack = ALBTestStack{}
	})
	AfterEach(func() {
		stack.Cleanup(ctx, tf)
	})
	Context("with ALB ip target configuration", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}

			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, lbcSpec, tgSpec)
				Expect(err).NotTo(HaveOccurred())
			})

			By("checking gateway status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})

			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})

			tgMap := map[string][]string{
				"80": {"HTTP"},
			}

			targetNumber := int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas)

			By("verifying AWS loadbalancer resources", func() {
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					TargetType:   "ip",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
					TargetGroups: tgMap,
					NumTargets:   targetNumber,
					TargetGroupHC: &verifier.TargetGroupHC{
						Protocol:           "HTTP",
						Port:               "traffic-port",
						Path:               "/",
						Interval:           15,
						Timeout:            5,
						HealthyThreshold:   3,
						UnhealthyThreshold: 3,
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("waiting for target group targets to be healthy", func() {
				err := verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, targetNumber)
				Expect(err).NotTo(HaveOccurred())
			})
			By("waiting until DNS name is available", func() {
				err := utils.WaitUntilDNSNameAvailable(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
			})
			By("sending http request to the lb", func() {
				url := fmt.Sprintf("http://%v/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
