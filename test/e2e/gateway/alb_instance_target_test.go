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
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strconv"
	"strings"
)

var _ = Describe("test k8s alb gateway using instance targets reconciled by the aws load balancer controller", func() {
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
	Context("with ALB instance target configuration with basic HTTPRoute", func() {
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
			httpr := buildHTTPRoute([]string{})
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec)
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
				strconv.Itoa(int(stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort)): {"HTTP"},
			}

			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:          "application",
					Scheme:        "internet-facing",
					TargetType:    "instance",
					Listeners:     stack.albResourceStack.getListenersPortMap(),
					TargetGroups:  tgMap,
					NumTargets:    len(nodeList),
					TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
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
			httpr := buildHTTPRoute([]string{testHostname})
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec)
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
				strconv.Itoa(int(stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort)): {"HTTP"},
			}

			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:          "application",
					Scheme:        "internet-facing",
					TargetType:    "instance",
					Listeners:     stack.albResourceStack.getListenersPortMap(),
					TargetGroups:  tgMap,
					NumTargets:    len(nodeList),
					TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
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
			httpr := buildHTTPRoute([]string{testHostname})
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec)
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
				strconv.Itoa(int(stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort)): {"HTTP"},
			}

			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:          "application",
					Scheme:        "internet-facing",
					TargetType:    "instance",
					Listeners:     stack.albResourceStack.getListenersPortMap(),
					TargetGroups:  tgMap,
					NumTargets:    len(nodeList),
					TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
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
			httpr := buildHTTPRoute([]string{testHostname})

			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec)
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
				strconv.Itoa(int(stack.albResourceStack.commonStack.svcs[0].Spec.Ports[0].NodePort)): {"HTTP"},
			}

			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:          "application",
					Scheme:        "internet-facing",
					TargetType:    "instance",
					Listeners:     stack.albResourceStack.getListenersPortMap(),
					TargetGroups:  tgMap,
					NumTargets:    len(nodeList),
					TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
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
