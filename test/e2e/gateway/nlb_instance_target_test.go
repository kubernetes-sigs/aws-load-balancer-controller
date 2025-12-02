package gateway

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
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

var _ = Describe("test nlb gateway using instance targets reconciled by the aws load balancer controller", func() {
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

	Context(fmt.Sprintf("with NLB instance target configuration, using readiness gates %+v", false), func() {
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

			instanceTargetType := elbv2gw.TargetTypeInstance
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &instanceTargetType,
				},
			}

			auxiliaryStack = newAuxiliaryResourceStack(ctx, tf, tgSpec, false)

			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, auxiliaryStack, lbcSpec, tgSpec, false)
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

				listenerPortMap := stack.nlbResourceStack.getListenersPortMap()
				// This listener _should_ not get materialized yet,
				// as the reference grant was not created.
				delete(listenerPortMap, strconv.Itoa(crossNamespacePort))

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					Listeners:    listenerPortMap,
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
			By("confirming the route status", func() {
				validateL4RouteStatusNotPermitted(tf, stack, hasTLS)
			})
			By("deploying ref grant", func() {
				err := auxiliaryStack.CreateReferenceGrants(ctx, tf, stack.nlbResourceStack.commonStack.ns)
				Expect(err).NotTo(HaveOccurred())
				// Give some time to have the listener get materialized.
				time.Sleep(2 * time.Minute)
			})
			By("ensuring cross namespace is materialized", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())

				// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{ // This TG is used by Listeners: TLS:443 (if enabled) and TCP:80 (always enabled)
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
					{ // This TG is used by Listeners: TCP:5000 (cross namespace route attached)
						Protocol:   "TCP",
						Port:       auxiliaryStack.svcs[0].Spec.Ports[0].NodePort,
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
			By("sending http request to the lb to the cross ns listener", func() {
				url := fmt.Sprintf("http://%v:5000/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
			By("confirming the route status", func() {
				validateL4RouteStatusPermitted(tf, stack, hasTLS)
			})
			By("removing ref grant", func() {
				err := auxiliaryStack.DeleteReferenceGrants(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				// Give some time to have the reference grant to be deleted
				time.Sleep(2 * time.Minute)
			})
			By("ensuring cross namespace listener is removed", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())

				// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{ // This TG is used by Listeners: TLS:443 (if enabled) and TCP:80 (always enabled)
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

				listenerPortMap := stack.nlbResourceStack.getListenersPortMap()
				// This listener _should_ be gone, as the reference grant is gone.
				delete(listenerPortMap, strconv.Itoa(crossNamespacePort))

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					Listeners:    listenerPortMap,
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("sending udp request to the lb", func() {
				endpoint := fmt.Sprintf("%v:8080", dnsName)
				err := tf.UDPVerifier.VerifyUDP(endpoint)
				Expect(err).NotTo(HaveOccurred())
			})
			By("confirming the route status", func() {
				validateL4RouteStatusNotPermitted(tf, stack, hasTLS)
			})
		})
	})

	Context(fmt.Sprintf("with NLB using no SG instance target configuration, using readiness gates %+v", false), func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme:               &interf,
				DisableSecurityGroup: awssdk.Bool(true),
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

			instanceTargetType := elbv2gw.TargetTypeInstance
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &instanceTargetType,
				},
			}

			auxiliaryStack = newAuxiliaryResourceStack(ctx, tf, tgSpec, false)

			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, auxiliaryStack, lbcSpec, tgSpec, false)
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

				listenerPortMap := stack.nlbResourceStack.getListenersPortMap()
				// This listener _should_ not get materialized yet,
				// as the reference grant was not created.
				delete(listenerPortMap, strconv.Itoa(crossNamespacePort))

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					Listeners:    listenerPortMap,
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
			By("confirming the route status", func() {
				validateL4RouteStatusNotPermitted(tf, stack, hasTLS)
			})
			By("deploying ref grant", func() {
				err := auxiliaryStack.CreateReferenceGrants(ctx, tf, stack.nlbResourceStack.commonStack.ns)
				Expect(err).NotTo(HaveOccurred())
				// Give some time to have the listener get materialized.
				time.Sleep(2 * time.Minute)
			})
			By("ensuring cross namespace is materialized", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())

				// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{ // This TG is used by Listeners: TLS:443 (if enabled) and TCP:80 (always enabled)
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
					{ // This TG is used by Listeners: TCP:5000 (cross namespace route attached)
						Protocol:   "TCP",
						Port:       auxiliaryStack.svcs[0].Spec.Ports[0].NodePort,
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
			By("sending http request to the lb to the cross ns listener", func() {
				url := fmt.Sprintf("http://%v:5000/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
			By("confirming the route status", func() {
				validateL4RouteStatusPermitted(tf, stack, hasTLS)
			})
			By("removing ref grant", func() {
				err := auxiliaryStack.DeleteReferenceGrants(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				// Give some time to have the reference grant to be deleted
				time.Sleep(2 * time.Minute)
			})
			By("ensuring cross namespace listener is removed", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())

				// TODO -- This might be hacky. Currently, the TCP svc always is 0, while UDP is 1.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{ // This TG is used by Listeners: TLS:443 (if enabled) and TCP:80 (always enabled)
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

				listenerPortMap := stack.nlbResourceStack.getListenersPortMap()
				// This listener _should_ be gone, as the reference grant is gone.
				delete(listenerPortMap, strconv.Itoa(crossNamespacePort))

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					Listeners:    listenerPortMap,
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("sending udp request to the lb", func() {
				endpoint := fmt.Sprintf("%v:8080", dnsName)
				err := tf.UDPVerifier.VerifyUDP(endpoint)
				Expect(err).NotTo(HaveOccurred())
			})
			By("confirming the route status", func() {
				validateL4RouteStatusNotPermitted(tf, stack, hasTLS)
			})
		})
		Context(fmt.Sprintf("with NLB instance target using TCP_UDP listener"), func() {
			BeforeEach(func() {})
			It("should provision internet-facing load balancer resources", func() {
				interf := elbv2gw.LoadBalancerSchemeInternetFacing
				lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
					Scheme: &interf,
				}

				instanceTargetType := elbv2gw.TargetTypeInstance
				tgSpec := elbv2gw.TargetGroupConfigurationSpec{
					DefaultConfiguration: elbv2gw.TargetGroupProps{
						TargetType: &instanceTargetType,
					},
				}
				By("deploying stack", func() {
					err := stack.DeployTCP_UDP(ctx, tf, lbcSpec, tgSpec, false)
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
					expectedTargetGroups := []verifier.ExpectedTargetGroup{
						{
							Protocol:   "TCP_UDP",
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
					}

					listenerPortMap := map[string]string{
						"80": "TCP_UDP",
					}

					err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
						Type:         "network",
						Scheme:       "internet-facing",
						Listeners:    listenerPortMap,
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
				By("sending udp request to the lb", func() {
					endpoint := fmt.Sprintf("%v:80", dnsName)
					err := tf.UDPVerifier.VerifyUDP(endpoint)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context(fmt.Sprintf("with NLB instance target configuration, using weighted listener"), func() {
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

					ipTargetType := elbv2gw.TargetTypeInstance
					tgSpec := elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{
							TargetType: &ipTargetType,
						},
					}

					By("deploying stack", func() {
						err := stack.DeployTCPWeightedStack(ctx, tf, lbcSpec, tgSpec, false)
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
					nodeList, err := stack.GetWorkerNodes(ctx, tf)
					Expect(err).ToNot(HaveOccurred())
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
							Protocol:   "TCP",
							Port:       stack.nlbResourceStack.commonStack.svcs[1].Spec.Ports[0].NodePort,
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

					By("verifying AWS loadbalancer resources", func() {
						err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
							Type:         "network",
							Scheme:       "internet-facing",
							Listeners:    stack.nlbResourceStack.getListenersPortMap(),
							TargetGroups: expectedTargetGroups,
						})
						Expect(err).NotTo(HaveOccurred())
					})
					By("waiting for target group targets to be healthy", func() {
						err := verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, len(nodeList))
						Expect(err).NotTo(HaveOccurred())
					})
					By("waiting until DNS name is available", func() {
						err := utils.WaitUntilDNSNameAvailable(ctx, dnsName)
						Expect(err).NotTo(HaveOccurred())
					})
					By("sending http request to the lb", func() {
						weightedRequestValidation(tf, fmt.Sprintf("http://%v/any-path", dnsName))
					})

					By("sending https request to the lb", func() {
						if hasTLS {
							weightedRequestValidation(tf, fmt.Sprintf("https://%v/any-path", dnsName))
						}
					})
				})
			})
		})
	})
})
