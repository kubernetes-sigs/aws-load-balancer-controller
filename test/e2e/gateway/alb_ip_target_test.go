package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/grpc/echo"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
	"time"
)

var _ = Describe("test k8s alb gateway using ip targets reconciled by the aws load balancer controller", func() {
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
	Context("with ALB ip target configuration with basic HTTPRoute", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme:                 &interf,
				ListenerConfigurations: listenerConfigurationForHeaderModification,
			}
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
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
				err := stack.DeployHTTP(ctx, auxiliaryStack, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, true)
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
			targetNumber := int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas)

			By("verifying AWS loadbalancer resources", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
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
			By("cross-ns listener should return 503 as no ref grant is available", func() {
				url := fmt.Sprintf("http://%v:5000/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(503))
				Expect(err).NotTo(HaveOccurred())
			})
			By("deploying ref grant", func() {
				err := auxiliaryStack.CreateReferenceGrants(ctx, tf, stack.albResourceStack.commonStack.ns)
				Expect(err).NotTo(HaveOccurred())
				// Give some time to have the listener get materialized.
				time.Sleep(2 * time.Minute)
			})
			By("ensuring cross namespace is materialized", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*auxiliaryStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("sending http request cross namespace service", func() {
				url := fmt.Sprintf("http://%v:5000/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
			By("removing ref grant", func() {
				err := auxiliaryStack.DeleteReferenceGrants(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				// Give some time to have the reference grant to be deleted
				time.Sleep(2 * time.Minute)
			})
			By("cross-ns listener should return 503 as no ref grant is available", func() {
				url := fmt.Sprintf("http://%v:5000/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(503))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with ALB ip target configuration with HTTPRoute specified matches", func() {
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
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "test-listener",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
			}

			httpr := buildHTTPRoute([]string{}, httpRouteRuleWithMatchesAndTargetGroupWeights, nil)
			By("deploying stack", func() {
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, true)
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
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
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
				err := verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas))
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

	Context("with ALB ip target configuration with HTTPRoute specified filter", func() {
		BeforeEach(func() {})
		It("should redirect requests correctly", func() {
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
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "test-listener",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
			}
			httpr := buildHTTPRoute([]string{}, httpRouteRuleWithMatchesAndFilters, nil)

			By("deploying stack", func() {
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, true)
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

	Context("with ALB ip target configuration with secure HTTPRoute", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}

			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			// Use the first certificate from the provided list
			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}

			// Set target type to IP
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
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
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, true)
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

			// For IP targets, we need to check the number of pod replicas
			targetNumber := int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas)

			expectedTargetGroups := []verifier.ExpectedTargetGroup{
				{
					Protocol:      "HTTP",
					Port:          80,
					NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
					TargetType:    "ip",
					TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
				},
			}

			By("verifying AWS loadbalancer resources", func() {
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
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

	Context("with ALB ip target configuration with secure HTTPRoute and mutual authentication PASSTHROUGH mode", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}

			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			// Use the first certificate from the provided list
			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: "passthrough",
				},
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}

			// Set target type to IP
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
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
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, true)
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

			// For IP targets, we need to check the number of pod replicas
			targetNumber := int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas)

			By("verifying AWS loadbalancer resources", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
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
				err := verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, targetNumber)
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

	Context("with ALB ip target configuration with secure HTTPRoute and and authenticate cognito action", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer with authenticate-cognito action", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}
			// Skip test if Cognito options not provided (similar to certificate check)
			if len(tf.Options.CognitoUserPoolArn) == 0 ||
				len(tf.Options.CognitoUserPoolClientId) == 0 ||
				len(tf.Options.CognitoUserPoolDomain) == 0 {
				Skip("Skipping authenticate-cognito tests, Cognito configuration not specified")
			}

			// Setup HTTPS listener with certificate
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			// Use the first certificate from the provided list
			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}

			// Set target type to IP
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}
			gwListeners := []gwv1.Listener{
				{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					Hostname: (*gwv1.Hostname)(awssdk.String(testHostname)),
				},
			}
			// Create ListenerRuleConfiguration with real Cognito values
			authenticateBehavior := elbv2gw.AuthenticateCognitoActionConditionalBehaviorEnumAuthenticate
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{
				Actions: []elbv2gw.Action{
					{
						Type: elbv2gw.ActionTypeAuthenticateCognito,
						AuthenticateCognitoConfig: &elbv2gw.AuthenticateCognitoActionConfig{
							UserPoolArn:      tf.Options.CognitoUserPoolArn,
							UserPoolClientID: tf.Options.CognitoUserPoolClientId,
							UserPoolDomain:   tf.Options.CognitoUserPoolDomain,
							Scope:            awssdk.String("openid"),
							AuthenticationRequestExtraParams: &map[string]string{
								"key1": "value1",
							},
							OnUnauthenticatedRequest: &authenticateBehavior,
							SessionCookieName:        awssdk.String("my-session-cookie"),
							SessionTimeout:           awssdk.Int64(604800),
						},
					},
				},
			}
			httpRouteRules := []gwv1.HTTPRouteRule{
				{
					BackendRefs: DefaultHttpRouteRuleBackendRefs,
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Name:  defaultLRConfigName,
								Kind:  constants.ListenerRuleConfiguration,
								Group: constants.ControllerCRDGroupVersion,
							},
						},
					},
				},
			}
			httpr := buildHTTPRoute([]string{testHostname}, httpRouteRules, nil)

			By("deploying stack", func() {
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, false)
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
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
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
									Values: []string{"/*"},
								},
							},
							{
								Field: awssdk.String(string(elbv2model.RuleConditionFieldHostHeader)),
								HostHeaderConfig: &elbv2types.HostHeaderConditionConfig{
									Values: []string{testHostname},
								},
							},
						},
						Actions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum(elbv2model.ActionTypeAuthenticateCognito),
								AuthenticateCognitoConfig: &elbv2types.AuthenticateCognitoActionConfig{
									UserPoolArn:      awssdk.String(tf.Options.CognitoUserPoolArn),
									UserPoolClientId: awssdk.String(tf.Options.CognitoUserPoolClientId),
									UserPoolDomain:   awssdk.String(tf.Options.CognitoUserPoolDomain),
									Scope:            awssdk.String("openid"),
									AuthenticationRequestExtraParams: map[string]string{
										"key1": "value1",
									},
									OnUnauthenticatedRequest: elbv2types.AuthenticateCognitoActionConditionalBehaviorEnumAuthenticate,
									SessionCookieName:        awssdk.String("my-session-cookie"),
									SessionTimeout:           awssdk.Int64(604800),
								},
							},
							{
								Type: elbv2types.ActionTypeEnum(elbv2model.ActionTypeForward),
								ForwardConfig: &elbv2types.ForwardActionConfig{
									TargetGroups: []elbv2types.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String(testTargetGroupArn),
											Weight:         awssdk.Int32(1),
										},
									},
								},
							},
						},
						Priority: 1,
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("waiting until DNS name is available", func() {
				err := utils.WaitUntilDNSNameAvailable(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying authenticate-cognito redirect for unauthenticated request", func() {
				url := fmt.Sprintf("https://%v/any-path", dnsName)
				urlOptions := http.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname,
					FollowRedirects:    false, // Don't follow redirects automatically
				}

				// Expect 302 redirect to Cognito
				err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, http.ResponseCodeMatches(302))
				Expect(err).NotTo(HaveOccurred())

				// Verify redirect Location header contains Cognito domain
				err = tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions,
					http.ResponseHeaderContains("Location", tf.Options.CognitoUserPoolDomain))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with both basic and secure HTTPRoutes", func() {
		BeforeEach(func() {})
		It("\"should provision internet-facing load balancer with both HTTP and HTTPS endpoints", func() {
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

			// Set target type to IP
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
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

			httpr := buildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, nil)

			By("deploying stack", func() {
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, true)
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

			// For IP targets, we need to check the number of pod replicas
			targetNumber := int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas)

			expectedTargetGroups := []verifier.ExpectedTargetGroup{
				{
					Protocol:      "HTTP",
					Port:          80,
					NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
					TargetType:    "ip",
					TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
				},
			}

			By("verifying AWS loadbalancer resources", func() {
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
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
				err := verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, targetNumber)
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

	Context("with ALB ip target configuration with GRPC", func() {
		It("should provision internet-facing load balancer resources", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}

			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			// Use the first certificate from the provided list
			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "test-listener",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
				},
			}

			grpcRouteRules := []gwv1.GRPCRouteRule{
				{
					BackendRefs: DefaultGrpcRouteRuleBackendRefs,
				},
				{
					Matches: []gwv1.GRPCRouteMatch{
						{
							Headers: []gwv1.GRPCHeaderMatch{
								{
									Type:  (*gwv1.GRPCHeaderMatchType)(awssdk.String("Exact")),
									Name:  "my-header",
									Value: "my-header-value",
								},
							},
						},
					},
					BackendRefs: []gwv1.GRPCBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: grpcDefaultName + "-other",
									Port: &defaultGrpcPort,
								},
							},
						},
					},
				},
			}

			grpcr := buildGRPCRoute([]string{}, grpcRouteRules, &gwListeners[0].Name)
			By("deploying stack", func() {
				err := stack.DeployGRPC(ctx, tf, gwListeners, []*gwv1.GRPCRoute{grpcr}, lbcSpec, tgSpec, lrcSpec, true)
				Expect(err).NotTo(HaveOccurred())
			})

			By("checking gateway status for lb dns name", func() {
				time.Sleep(2 * time.Minute)
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})
			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})

			targetNumber := int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas)

			By("verifying AWS loadbalancer resources", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          50051,
						NumTargets:    int(*stack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC_GRPC,
					},
					{
						Protocol:      "HTTP",
						Port:          50051,
						NumTargets:    int(*stack.albResourceStack.commonStack.dps[1].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC_GRPC,
					},
				}
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internet-facing",
					Listeners:    stack.albResourceStack.getListenersPortMap(),
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
			By("sending grpc request to the lb", func() {
				target := fmt.Sprintf("%s:443", dnsName)
				tlsConfig := &tls.Config{
					InsecureSkipVerify: true, // This skips all certificate verification, including expiry.
				}

				conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
				Expect(err).NotTo(HaveOccurred())
				c := echo.NewEchoServiceClient(conn)

				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"foo": "cat"}))
				response, err := c.Echo(mdCtx, &echo.EchoRequest{Message: "Hello from E2E test"})
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Message).To(Equal("Hello from E2E test"))
			})
			By("sending grpc request with certain header must forward traffic to right backend", func() {
				target := fmt.Sprintf("%s:443", dnsName)
				tlsConfig := &tls.Config{
					InsecureSkipVerify: true, // This skips all certificate verification, including expiry.
				}

				conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
				Expect(err).NotTo(HaveOccurred())
				c := echo.NewEchoServiceClient(conn)

				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"my-header": "my-header-value"}))
				response, err := c.FixedResponse(mdCtx, &echo.FixedResponseRequest{})
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Message).To(Equal("Hello World - Other"))
			})
			By("sending grpc request with header missing uses default service.", func() {
				target := fmt.Sprintf("%s:443", dnsName)
				tlsConfig := &tls.Config{
					InsecureSkipVerify: true, // This skips all certificate verification, including expiry.
				}

				conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
				Expect(err).NotTo(HaveOccurred())
				c := echo.NewEchoServiceClient(conn)

				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{}))
				response, err := c.FixedResponse(mdCtx, &echo.FixedResponseRequest{})
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Message).To(Equal("Hello World"))
			})
			By("update grpc route to remove default rule", func() {

				err := tf.K8sClient.Get(ctx, types.NamespacedName{Name: stack.albResourceStack.grpcrs[0].Name, Namespace: stack.albResourceStack.grpcrs[0].Namespace}, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())

				stack.albResourceStack.grpcrs[0].Spec.Rules = []gwv1.GRPCRouteRule{
					{
						Matches: []gwv1.GRPCRouteMatch{
							{
								Headers: []gwv1.GRPCHeaderMatch{
									{
										Type:  (*gwv1.GRPCHeaderMatchType)(awssdk.String("Exact")),
										Name:  "my-header",
										Value: "my-header-value",
									},
								},
							},
						},
						BackendRefs: []gwv1.GRPCBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: grpcDefaultName + "-other",
										Port: &defaultGrpcPort,
									},
								},
							},
						},
					},
				}

				err = stack.albResourceStack.updateGRPCRoute(ctx, tf, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())
				// Wait for listener change to propagate.
				time.Sleep(1 * time.Minute)
			})
			By("send grpc request with correct request header", func() {
				c, err := generateGRPCClient(dnsName)
				Expect(err).NotTo(HaveOccurred())
				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{"my-header": "my-header-value"}))
				response, err := c.FixedResponse(mdCtx, &echo.FixedResponseRequest{})
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Message).To(Equal("Hello World - Other"))
			})
			By("sending grpc request with header missing uses default service.", func() {
				c, err := generateGRPCClient(dnsName)
				Expect(err).NotTo(HaveOccurred())
				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{}))
				_, err = c.FixedResponse(mdCtx, &echo.FixedResponseRequest{})
				Expect(err).To(HaveOccurred())
			})
			By("update grpc route to route by service / method name. use invalid service", func() {

				err := tf.K8sClient.Get(ctx, types.NamespacedName{Name: stack.albResourceStack.grpcrs[0].Name, Namespace: stack.albResourceStack.grpcrs[0].Namespace}, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())

				stack.albResourceStack.grpcrs[0].Spec.Rules = []gwv1.GRPCRouteRule{
					{
						Matches: []gwv1.GRPCRouteMatch{
							{
								Method: &gwv1.GRPCMethodMatch{
									Service: awssdk.String("com.example.FakeService"),
									Method:  awssdk.String("FakeMethod"),
								},
							},
						},
						BackendRefs: []gwv1.GRPCBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: grpcDefaultName + "-other",
										Port: &defaultGrpcPort,
									},
								},
							},
						},
					},
				}

				err = stack.albResourceStack.updateGRPCRoute(ctx, tf, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())
				// Wait for listener change to propagate.
				time.Sleep(1 * time.Minute)
			})
			By("sending grpc request should result in a failure.", func() {
				c, err := generateGRPCClient(dnsName)
				Expect(err).NotTo(HaveOccurred())
				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{}))
				_, err = c.FixedResponse(mdCtx, &echo.FixedResponseRequest{})
				Expect(err).To(HaveOccurred())
			})
			By("update grpc route to route by service / method name. filter by service", func() {

				err := tf.K8sClient.Get(ctx, types.NamespacedName{Name: stack.albResourceStack.grpcrs[0].Name, Namespace: stack.albResourceStack.grpcrs[0].Namespace}, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())

				stack.albResourceStack.grpcrs[0].Spec.Rules = []gwv1.GRPCRouteRule{
					{
						Matches: []gwv1.GRPCRouteMatch{
							{
								Method: &gwv1.GRPCMethodMatch{
									Service: awssdk.String("echo.EchoService"),
								},
							},
						},
						BackendRefs: []gwv1.GRPCBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: grpcDefaultName + "-other",
										Port: &defaultGrpcPort,
									},
								},
							},
						},
					},
				}

				err = stack.albResourceStack.updateGRPCRoute(ctx, tf, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())
				// Wait for listener change to propagate.
				time.Sleep(1 * time.Minute)
			})
			By("sending grpc request should work for both methods", func() {
				c, err := generateGRPCClient(dnsName)
				Expect(err).NotTo(HaveOccurred())
				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{}))
				_, err = c.FixedResponse(mdCtx, &echo.FixedResponseRequest{})
				Expect(err).ToNot(HaveOccurred())

				_, err = c.Echo(mdCtx, &echo.EchoRequest{Message: "foo"})
				Expect(err).ToNot(HaveOccurred())
			})
			By("update grpc route to route by service / method name. filter by service and method", func() {

				err := tf.K8sClient.Get(ctx, types.NamespacedName{Name: stack.albResourceStack.grpcrs[0].Name, Namespace: stack.albResourceStack.grpcrs[0].Namespace}, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())

				stack.albResourceStack.grpcrs[0].Spec.Rules = []gwv1.GRPCRouteRule{
					{
						Matches: []gwv1.GRPCRouteMatch{
							{
								Method: &gwv1.GRPCMethodMatch{
									Service: awssdk.String("echo.EchoService"),
									Method:  awssdk.String("Echo"),
								},
							},
						},
						BackendRefs: []gwv1.GRPCBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: grpcDefaultName + "-other",
										Port: &defaultGrpcPort,
									},
								},
							},
						},
					},
				}

				err = stack.albResourceStack.updateGRPCRoute(ctx, tf, stack.albResourceStack.grpcrs[0])
				Expect(err).NotTo(HaveOccurred())
				// Wait for listener change to propagate.
				time.Sleep(1 * time.Minute)
			})
			By("sending grpc request should work for Echo method, should fail for FixedResponse", func() {
				c, err := generateGRPCClient(dnsName)
				Expect(err).NotTo(HaveOccurred())
				mdCtx := metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{}))
				_, err = c.FixedResponse(mdCtx, &echo.FixedResponseRequest{})
				Expect(err).To(HaveOccurred())

				_, err = c.Echo(mdCtx, &echo.EchoRequest{Message: "foo"})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
