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
	"strings"
)

var _ = Describe("test nlb gateway using instance targets reconciled by the aws load balancer controller", func() {
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
		ctx = context.Background()
		stack = NLBTestStack{}
	})
	AfterEach(func() {
		stack.Cleanup(ctx, tf)
	})
	for _, readinessGateEnabled := range []bool{true, false} {
		Context(fmt.Sprintf("with NLB instance target configuration, using readiness gates %+v", readinessGateEnabled), func() {
			BeforeEach(func() {})
			It("should provision internet-facing load balancer resources", func() {
				interf := elbv2gw.LoadBalancerSchemeInternetFacing
				lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
					Scheme: &interf,
				}

				var hasTLS bool
				if len(tf.Options.CertificateARNs) > 0 {
					cert := strings.Split(tf.Options.CertificateARNs, ",")[0]

					lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{
						{
							DefaultCertificate: &cert,
							ProtocolPort:       "TLS:443",
						},
					}
					hasTLS = true
				}

				tgSpec := elbv2gw.TargetGroupConfigurationSpec{}
				By("deploying stack", func() {
					err := stack.Deploy(ctx, tf, nil, lbcSpec, tgSpec, readinessGateEnabled)
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

				By("verifying AWS loadbalancer resources", func() {
					nodeList, err := stack.GetWorkerNodes(ctx, tf)
					Expect(err).ToNot(HaveOccurred())

					// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
					expectedTargetGroups := []verifier.ExpectedTargetGroup{
						{
							Protocol:   "TCP",
							Port:       stack.nlbResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort,
							NumTargets: len(nodeList),
							TargetType: "instance",
							TargetGroupHC: &verifier.TargetGroupHC{
								Protocol:           "TCP",
								Port:               "traffic-port",
								Interval:           15,
								Timeout:            5,
								HealthyThreshold:   3,
								UnhealthyThreshold: 3,
							},
						},
						{
							Protocol:   "UDP",
							Port:       stack.nlbResourceStack.commonStack.svcs[1].Spec.Ports[1].NodePort,
							NumTargets: len(nodeList),
							TargetType: "instance",
							TargetGroupHC: &verifier.TargetGroupHC{
								Protocol:           "TCP",
								Port:               "traffic-port",
								Interval:           15,
								Timeout:            5,
								HealthyThreshold:   3,
								UnhealthyThreshold: 3,
							},
						},
					}

					err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
						Type:         "network",
						Scheme:       "internet-facing",
						Listeners:    stack.nlbResourceStack.getListenersPortMap(),
						TargetGroups: expectedTargetGroups,
					})
					Expect(err).NotTo(HaveOccurred())
				})
				By("waiting for target group targets to be healthy", func() {
					nodeList, err := stack.GetWorkerNodes(ctx, tf)
					Expect(err).ToNot(HaveOccurred())
					err = verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, len(nodeList))
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
				By("sending https request to the lb", func() {
					if hasTLS {
						url := fmt.Sprintf("https://%v/any-path", dnsName)
						err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
						Expect(err).NotTo(HaveOccurred())
					}
				})
				By("sending udp request to the lb", func() {
					endpoint := fmt.Sprintf("%v:8080", dnsName)
					err := tf.UDPVerifier.VerifyUDP(endpoint)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
	}
})
