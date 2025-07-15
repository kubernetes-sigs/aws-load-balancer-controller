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
	"strconv"
	"strings"
	"time"
)

var _ = Describe("test nlb gateway using ip targets reconciled by the aws load balancer controller", func() {
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
		auxiliaryStack.Cleanup(ctx, tf)
	})
	for _, readinessGateEnabled := range []bool{true, false} {
		Context(fmt.Sprintf("with NLB ip target configuration, using readiness gates %+v", readinessGateEnabled), func() {
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

				ipTargetType := elbv2gw.TargetTypeIP
				tgSpec := elbv2gw.TargetGroupConfigurationSpec{
					DefaultConfiguration: elbv2gw.TargetGroupProps{
						TargetType: &ipTargetType,
					},
				}

				auxiliaryStack = newAuxiliaryResourceStack(ctx, tf, tgSpec, readinessGateEnabled)

				By("deploying stack", func() {

					err := stack.Deploy(ctx, tf, auxiliaryStack, lbcSpec, tgSpec, readinessGateEnabled)
					Expect(err).NotTo(HaveOccurred())

					err = auxiliaryStack.Deploy(ctx, tf)
					Expect(err).NotTo(HaveOccurred())

					fmt.Println("STARTING SLEEP!!!")
					time.Sleep(2 * time.Minute)
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

				// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{ // This TG is used by Listeners: TLS:443 (if enabled) and TCP:80 (always enabled)
						Protocol:   "TCP",
						Port:       80,
						NumTargets: targetNumber,
						TargetType: "ip",
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
						Port:       8080,
						NumTargets: targetNumber,
						TargetType: "ip",
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

				listenerPortMap := stack.nlbResourceStack.getListenersPortMap()
				// This listener _should_ not get materialized yet,
				// as the reference grant was not created.
				delete(listenerPortMap, strconv.Itoa(crossNamespacePort))

				By("verifying AWS loadbalancer resources", func() {
					err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
						Type:         "network",
						Scheme:       "internet-facing",
						Listeners:    listenerPortMap,
						TargetGroups: expectedTargetGroups,
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
				By("deploying ref grant", func() {
					err := auxiliaryStack.CreateReferenceGrants(ctx, tf, stack.nlbResourceStack.commonStack.ns)
					Expect(err).NotTo(HaveOccurred())
					// Give some time to have the listener get materialized.
					time.Sleep(2 * time.Minute)
				})
				By("ensuring cross namespace is materialized", func() {
					// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
					expectedTargetGroups := []verifier.ExpectedTargetGroup{
						{ // This TG is used by Listeners: TLS:443 (if enabled) and TCP:80 (always enabled)
							Protocol:   "TCP",
							Port:       80,
							NumTargets: targetNumber,
							TargetType: "ip",
							TargetGroupHC: &verifier.TargetGroupHC{
								Protocol:           "TCP",
								Port:               "traffic-port",
								Interval:           15,
								Timeout:            5,
								HealthyThreshold:   3,
								UnhealthyThreshold: 3,
							},
						},
						{ // This TG is used by Listeners: TCP:5000 (cross-ns route)
							Protocol:   "TCP",
							Port:       80,
							NumTargets: targetNumber,
							TargetType: "ip",
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
							Port:       8080,
							NumTargets: targetNumber,
							TargetType: "ip",
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

					err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
						Type:         "network",
						Scheme:       "internet-facing",
						Listeners:    stack.nlbResourceStack.getListenersPortMap(),
						TargetGroups: expectedTargetGroups,
					})
					Expect(err).NotTo(HaveOccurred())
				})
				By("removing ref grant", func() {
					err := auxiliaryStack.DeleteReferenceGrants(ctx, tf)
					Expect(err).NotTo(HaveOccurred())
					// Give some time to have the reference grant to be deleted
					time.Sleep(2 * time.Minute)
				})
				By("ensuring cross namespace listener is removed", func() {
					// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
					expectedTargetGroups := []verifier.ExpectedTargetGroup{
						{ // This TG is used by Listeners: TLS:443 (if enabled) and TCP:80 (always enabled)
							Protocol:   "TCP",
							Port:       80,
							NumTargets: targetNumber,
							TargetType: "ip",
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
							Port:       8080,
							NumTargets: targetNumber,
							TargetType: "ip",
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

					listenerPortMap := stack.nlbResourceStack.getListenersPortMap()
					// This listener _should_ not get materialized yet,
					// as the reference grant was not created.
					delete(listenerPortMap, strconv.Itoa(crossNamespacePort))

					err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
						Type:         "network",
						Scheme:       "internet-facing",
						Listeners:    listenerPortMap,
						TargetGroups: expectedTargetGroups,
					})
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
	}
})
