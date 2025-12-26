package gateway

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
)

var _ = Describe("test nlb gateway with QUIC protocol support", func() {
	var (
		ctx            context.Context
		stack          NLBTestStack
		auxiliaryStack *auxiliaryResourceStack
		dnsName        string
		lbARN          string
	)

	BeforeEach(func() {
		if !tf.Options.EnableGatewayTests {
			Skip("Skipping gateway tests")
		}
		ctx = context.Background()
		stack = NLBTestStack{}
		auxiliaryStack = nil
	})

	AfterEach(func() {
		stack.Cleanup(ctx, tf)
		if auxiliaryStack != nil {
			auxiliaryStack.Cleanup(ctx, tf)
		}
	})

	Context("with QUIC protocol enabled", func() {
		It("should provision NLB with QUIC protocol for UDP listeners", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			quicEnabled := true
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
				ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
					{
						ProtocolPort: "UDP:8080",
						QuicEnabled:  &quicEnabled,
					},
				},
			}

			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}

			auxiliaryStack = newAuxiliaryResourceStack(ctx, tf, tgSpec, false)

			By("deploying stack with QUIC enabled", func() {
				err := stack.DeployQUIC(ctx, tf, lbcSpec, tgSpec, false)
				Expect(err).NotTo(HaveOccurred())

				err = auxiliaryStack.Deploy(ctx, tf)
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

			targetNumber := int(*stack.nlbResourceStack.commonStack.dps[0].Spec.Replicas)

			expectedTargetGroups := []verifier.ExpectedTargetGroup{
				{
					Protocol:   "QUIC", // UDP should be upgraded to QUIC
					Port:       8080,
					NumTargets: targetNumber,
					TargetType: "ip",
				},
			}

			By("verifying target groups have QUIC protocol", func() {
				err := tf.LBManager.VerifyTargetGroups(ctx, lbARN, expectedTargetGroups)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying listeners have QUIC protocol", func() {
				expectedListeners := []verifier.ExpectedListener{
					{
						Port:     8080,
						Protocol: "QUIC",
					},
				}
				err := tf.LBManager.VerifyListeners(ctx, lbARN, expectedListeners)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("should provision NLB with TCP_QUIC protocol for TCP_UDP listeners", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			quicEnabled := true
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
				ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
					{
						ProtocolPort: "TCP_UDP:8080",
						QuicEnabled:  &quicEnabled,
					},
				},
			}

			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}

			auxiliaryStack = newAuxiliaryResourceStack(ctx, tf, tgSpec, false)

			By("deploying stack with QUIC enabled for TCP_UDP", func() {
				err := stack.DeployTCP_UDP_QUIC(ctx, tf, lbcSpec, tgSpec, false)
				Expect(err).NotTo(HaveOccurred())

				err = auxiliaryStack.Deploy(ctx, tf)
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

			targetNumber := int(*stack.nlbResourceStack.commonStack.dps[0].Spec.Replicas)

			expectedTargetGroups := []verifier.ExpectedTargetGroup{
				{
					Protocol:   "TCP_QUIC", // TCP_UDP should be upgraded to TCP_QUIC
					Port:       8080,
					NumTargets: targetNumber,
					TargetType: "ip",
				},
			}

			By("verifying target groups have TCP_QUIC protocol", func() {
				err := tf.LBManager.VerifyTargetGroups(ctx, lbARN, expectedTargetGroups)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying listeners have TCP_QUIC protocol", func() {
				expectedListeners := []verifier.ExpectedListener{
					{
						Port:     8080,
						Protocol: "TCP_QUIC",
					},
				}
				err := tf.LBManager.VerifyListeners(ctx, lbARN, expectedListeners)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
