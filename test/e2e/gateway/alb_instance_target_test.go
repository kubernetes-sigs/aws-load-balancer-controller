package gateway

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
	"time"
)

var _ = Describe("test k8s alb gateway using instance targets reconciled by the aws load balancer controller", func() {
	var (
		ctx            context.Context
		stack          ALBTestStack
		auxiliaryStack *auxiliaryResourceStack
		dnsName        string
		lbARN          string
	)
	BeforeEach(func() {
		if !tf.Options.EnableGatewayTests {
			Skip("Skipping gateway tests")
		}
		ctx = context.Background()
		stack = ALBTestStack{}
		auxiliaryStack = nil
	})
	AfterEach(func() {
		stack.Cleanup(ctx, tf)
		if auxiliaryStack != nil {
			auxiliaryStack.Cleanup(ctx, tf)
		}
	})
	Context("with ALB instance target configuration with basic HTTPRoute", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme:                 &interf,
				ListenerConfigurations: listenerConfigurationForHeaderModification,
			}
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "test-listener",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
			}
			auxiliaryStack = newAuxiliaryResourceStack(ctx, tf, tgSpec, true)
			httpr := buildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, &gwListeners[0].Name)
			By("deploying stack", func() {
				err := stack.Deploy(ctx, auxiliaryStack, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, true)
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
			By("verifying listener attributes header modification is applied", func() {
				lsARN := verifier.GetLoadBalancerListenerARN(ctx, tf, lbARN, "80")
				err := verifier.VerifyListenerAttributes(ctx, tf, lsARN, map[string]string{
					headerModificationServerEnabled: "true",
					headerModificationMaxAge:        headerModificationMaxAgeValue,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort,
						NumTargets:    len(nodeList),
						TargetType:    "instance",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verifying HTTP load balancer listener", func() {
				err := verifier.VerifyLoadBalancerListener(ctx, tf, lbARN, int32(gwListeners[0].Port), &verifier.ListenerExpectation{
					ProtocolPort: "HTTP:80",
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
			By("cross-ns listener should return 404 as no ref grant is available", func() {
				url := fmt.Sprintf("http://%v:5000/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(404))
				Expect(err).NotTo(HaveOccurred())
			})
			By("deploying ref grant", func() {
				err := auxiliaryStack.CreateReferenceGrants(ctx, tf, stack.albResourceStack.commonStack.ns)
				Expect(err).NotTo(HaveOccurred())
				// Give some time to have the listener get materialized.
				time.Sleep(2 * time.Minute)
			})
			By("ensuring cross namespace is materialized", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort,
						NumTargets:    len(nodeList),
						TargetType:    "instance",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
					{
						Protocol:      "HTTP",
						Port:          auxiliaryStack.svcs[0].Spec.Ports[0].NodePort,
						NumTargets:    len(nodeList),
						TargetType:    "instance",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
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
			By("cross-ns listener should return 404 as no ref grant is available", func() {
				url := fmt.Sprintf("http://%v:5000/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(404))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
	Context("with ALB instance target configuration with HTTPRoute specified matches", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "test-listener",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
			}
			httpr := buildHTTPRoute([]string{}, httpRouteRuleWithMatchesAndTargetGroupWeights, nil)

			By("deploying stack", func() {
				err := stack.Deploy(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, true)
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
						Protocol:      "HTTP",
						Port:          stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort,
						NumTargets:    len(nodeList),
						TargetType:    "instance",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying HTTP load balancer listener", func() {
				err := verifier.VerifyLoadBalancerListener(ctx, tf, lbARN, int32(gwListeners[0].Port), &verifier.ListenerExpectation{
					ProtocolPort: "HTTP:80",
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verifying listener rules", func() {
				err := verifier.VerifyLoadBalancerListenerRules(ctx, tf, lbARN, int32(gwListeners[0].Port), []verifier.ListenerRuleExpectation{
					{
						Conditions: []elbv2types.RuleCondition{
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldPathPattern)),
								PathPatternConfig: &elbv2types.PathPatternConditionConfig{
									Values: []string{testPathString},
								},
							},
						},
						Actions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum(elbv2model.ActionTypeForward),
								ForwardConfig: &elbv2types.ForwardActionConfig{
									TargetGroups: []elbv2types.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String(testTargetGroupArn),
											Weight:         awssdk.Int32(50),
										},
									},
								},
							},
						},
						Priority: 1,
					},
					{
						Conditions: []elbv2types.RuleCondition{
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldPathPattern)),
								PathPatternConfig: &elbv2types.PathPatternConditionConfig{
									Values: []string{testPathString, fmt.Sprintf("%s/*", testPathString)},
								},
							},
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldHTTPRequestMethod)),
								HttpRequestMethodConfig: &elbv2types.HttpRequestMethodConditionConfig{
									Values: []string{
										"GET",
									},
								},
							},
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldHTTPHeader)),
								HttpHeaderConfig: &elbv2types.HttpHeaderConditionConfig{
									HttpHeaderName: awssdk.String(testHttpHeaderNameOne),
									Values: []string{
										testHttpHeaderValueOne,
									},
								},
							},
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldHTTPHeader)),
								HttpHeaderConfig: &elbv2types.HttpHeaderConditionConfig{
									HttpHeaderName: awssdk.String(testHttpHeaderNameTwo),
									Values: []string{
										testHttpHeaderValueTwo,
									},
								},
							},
						},
						Actions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum(elbv2model.ActionTypeForward),
								ForwardConfig: &elbv2types.ForwardActionConfig{
									TargetGroups: []elbv2types.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String(testTargetGroupArn),
											Weight:         awssdk.Int32(30),
										},
									},
								},
							},
						},
						Priority: 2,
					},
					{
						Conditions: []elbv2types.RuleCondition{
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldPathPattern)),
								PathPatternConfig: &elbv2types.PathPatternConditionConfig{
									Values: []string{testPathString, fmt.Sprintf("%s/*", testPathString)},
								},
							},
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldQueryString)),
								QueryStringConfig: &elbv2types.QueryStringConditionConfig{
									Values: []elbv2types.QueryStringKeyValuePair{
										{
											Key:   awssdk.String(testQueryStringKeyOne),
											Value: awssdk.String(testQueryStringValueOne),
										},
									},
								},
							},
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldQueryString)),
								QueryStringConfig: &elbv2types.QueryStringConditionConfig{
									Values: []elbv2types.QueryStringKeyValuePair{
										{
											Key:   awssdk.String(testQueryStringKeyTwo),
											Value: awssdk.String(testQueryStringValueTwo),
										},
									},
								},
							},
						},
						Actions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum(elbv2model.ActionTypeForward),
								ForwardConfig: &elbv2types.ForwardActionConfig{
									TargetGroups: []elbv2types.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String(testTargetGroupArn),
											Weight:         awssdk.Int32(30),
										},
									},
								},
							},
						},
						Priority: 3,
					},
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
				url := fmt.Sprintf("http://%v%s", dnsName, testPathString)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with ALB instance target configuration with HTTPRoute specified filter", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "test-listener",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
			}
			httpr := buildHTTPRoute([]string{}, httpRouteRuleWithMatchesAndFilters, nil)

			By("deploying stack", func() {
				err := stack.Deploy(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, true)
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

			By("waiting until DNS name is available", func() {
				err := utils.WaitUntilDNSNameAvailable(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
			})

			By("testing redirect with ReplaceFullPath", func() {
				httpExp := httpexpect.New(tf.LoggerReporter, fmt.Sprintf("http://%v", dnsName))
				httpExp.GET("/old-path").WithRedirectPolicy(httpexpect.DontFollowRedirects).Expect().
					Status(301).
					Header("Location").Equal("https://example.com:80/new-path")
			})

			By("testing redirect with ReplacePrefixMatch", func() {
				httpExp := httpexpect.New(tf.LoggerReporter, fmt.Sprintf("http://%v", dnsName))
				httpExp.GET("/api/v1/users").WithRedirectPolicy(httpexpect.DontFollowRedirects).Expect().
					Status(302).
					Header("Location").Equal("https://api.example.com:80/v2/*")
			})

			By("testing redirect with scheme and port change", func() {
				httpExp := httpexpect.New(tf.LoggerReporter, fmt.Sprintf("http://%v", dnsName))
				httpExp.GET("/secure").WithRedirectPolicy(httpexpect.DontFollowRedirects).Expect().
					Status(302).
					Header("Location").Equal("https://secure.example.com:8443/secure")
			})
		})
	})
	Context("with ALB instance target configuration with secure HTTPRoute", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					Hostname: (*gwv1.Hostname)(awssdk.String(testHostname)),
				},
			}
			httpr := buildHTTPRoute([]string{testHostname}, []gwv1.HTTPRouteRule{}, nil)
			By("deploying stack", func() {
				err := stack.Deploy(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, true)
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
						Protocol:      "HTTP",
						Port:          stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort,
						NumTargets:    len(nodeList),
						TargetType:    "instance",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verifying AWS load balancer listener", func() {
				err := verifier.VerifyLoadBalancerListener(ctx, tf, lbARN, int32(gwListeners[0].Port), &verifier.ListenerExpectation{
					ProtocolPort:          "HTTPS:443",
					DefaultCertificateARN: awssdk.ToString(lsConfig.DefaultCertificate),
					MutualAuthentication: &verifier.MutualAuthenticationExpectation{
						Mode: "off",
					},
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
			By("sending https request to the lb", func() {
				url := fmt.Sprintf("https://%v/any-path", dnsName)
				urlOptions := http.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname, // Set Host header
				}
				err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with ALB instance target configuration with secure HTTPRoute and mutual authentication PASSTHROUGH mode", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: "passthrough",
				},
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					Hostname: (*gwv1.Hostname)(awssdk.String(testHostname)),
				},
			}
			httpr := buildHTTPRoute([]string{testHostname}, []gwv1.HTTPRouteRule{}, nil)
			By("deploying stack", func() {
				err := stack.Deploy(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, true)
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
						Protocol:      "HTTP",
						Port:          stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort,
						NumTargets:    len(nodeList),
						TargetType:    "instance",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verifying AWS load balancer listener", func() {
				err := verifier.VerifyLoadBalancerListener(ctx, tf, lbARN, int32(gwListeners[0].Port), &verifier.ListenerExpectation{
					ProtocolPort:          "HTTPS:443",
					DefaultCertificateARN: awssdk.ToString(lsConfig.DefaultCertificate),
					MutualAuthentication: &verifier.MutualAuthenticationExpectation{
						Mode: "passthrough",
					},
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
			By("sending https request to the lb", func() {
				url := fmt.Sprintf("https://%v/any-path", dnsName)
				urlOptions := http.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname, // Set Host header
				}
				err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with both basic and secure HTTPRoutes", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer with both HTTP and HTTPS endpoints", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}

			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			// Configure both HTTP and HTTPS listeners
			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			httpLsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort: "HTTP:80",
			}
			httpsLsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{
				httpLsConfig,
				httpsLsConfig,
			}

			tgSpec := elbv2gw.TargetGroupConfigurationSpec{}

			gwListeners := []gwv1.Listener{
				{
					Name:     "http80",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					Hostname: (*gwv1.Hostname)(awssdk.String(testHostname)),
				},
			}
			httpr := buildHTTPRoute([]string{testHostname}, []gwv1.HTTPRouteRule{}, nil)

			By("deploying stack", func() {
				err := stack.Deploy(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, true)
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
						Protocol:      "HTTP",
						Port:          stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort,
						NumTargets:    len(nodeList),
						TargetType:    "instance",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			// Verify HTTP listener
			By("verifying HTTP load balancer listener", func() {
				err := verifier.VerifyLoadBalancerListener(ctx, tf, lbARN, int32(gwListeners[0].Port), &verifier.ListenerExpectation{
					ProtocolPort: "HTTP:80",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			// Verify HTTPS listener
			By("verifying HTTPS load balancer listener", func() {
				err := verifier.VerifyLoadBalancerListener(ctx, tf, lbARN, int32(gwListeners[1].Port), &verifier.ListenerExpectation{
					ProtocolPort:          "HTTPS:443",
					DefaultCertificateARN: awssdk.ToString(httpsLsConfig.DefaultCertificate),
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

			// Test HTTP endpoint
			By("sending HTTP request to the lb", func() {
				url := fmt.Sprintf("http://%v/any-path", dnsName)
				urlOptions := http.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname, // Set Host header
					Method:             "GET",
				}
				err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})

			// Test HTTPS endpoint
			By("sending HTTPS request to the lb", func() {
				url := fmt.Sprintf("https://%v/any-path", dnsName)
				urlOptions := http.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname, // Set Host header
					Method:             "GET",
				}
				err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})
