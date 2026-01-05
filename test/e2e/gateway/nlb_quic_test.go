package gateway

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
)

var _ = Describe("test nlb gateway with QUIC protocol support", func() {
	var (
		ctx     context.Context
		stack   NLBTestStack
		dnsName string
		lbARN   string
	)

	BeforeEach(func() {
		if !tf.Options.EnableGatewayTests {
			Skip("Skipping gateway tests")
		}

		if tf.Options.IPFamily == framework.IPv6 {
			Skip("QUIC does not support IPv6")
		}
		ctx = context.Background()
		stack = NLBTestStack{}
	})

	AfterEach(func() {
		stack.Cleanup(ctx, tf)
	})

	Context("with QUIC protocol enabled", func() {
		It("should provision NLB with QUIC protocol for UDP listeners", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			quicEnabled := true
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme:               &interf,
				DisableSecurityGroup: awssdk.Bool(true),
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
			By("deploying stack with QUIC enabled", func() {
				err := stack.DeployQUIC(ctx, tf, lbcSpec, tgSpec, map[string]string{
					"elbv2.k8s.aws/quic-server-id-inject": "enabled",
				})
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
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"8080": "QUIC",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("should provision NLB with TCP_QUIC protocol for TCP_UDP listeners", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			quicEnabled := true
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme:               &interf,
				DisableSecurityGroup: awssdk.Bool(true),
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

			By("deploying stack with QUIC enabled for TCP_UDP", func() {
				err := stack.DeployTCP_QUIC(ctx, tf, lbcSpec, tgSpec, map[string]string{
					"elbv2.k8s.aws/quic-server-id-inject": "enabled",
				})
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
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"8080": "TCP_QUIC",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
