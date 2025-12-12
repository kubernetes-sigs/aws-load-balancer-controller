package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("test combined ALB and NLB gateways with HTTPRoute and TCPRoute", func() {
	var (
		ctx      context.Context
		albStack ALBTestStack
		nlbStack NLBTestStack
	)

	BeforeEach(func() {
		if !tf.Options.EnableGatewayTests {
			Skip("Skipping gateway tests")
		}
		ctx = context.Background()
		albStack = ALBTestStack{}
		nlbStack = NLBTestStack{}
	})

	AfterEach(func() {
		albStack.Cleanup(ctx, tf)
		nlbStack.Cleanup(ctx, tf)
	})

	Context("with ALB and NLB gateways using IP targets", func() {
		var albDnsName string
		var albARN string
		var nlbDnsName string
		var nlbARN string
		var refGrant *gwbeta1.ReferenceGrant
		It("should provision both ALB and NLB load balancers with HTTPRoute and TCPRoute", func() {
			// ALB Configuration
			albInterf := elbv2gw.LoadBalancerSchemeInternal
			albLbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &albInterf,
			}

			// NLB Configuration
			nlbInterf := elbv2gw.LoadBalancerSchemeInternetFacing
			nlbLbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &nlbInterf,
			}

			// Configure TLS for both if certificates are available
			var hasTLS bool
			if len(tf.Options.CertificateARNs) > 0 {
				cert := strings.Split(tf.Options.CertificateARNs, ",")[0]

				// ALB HTTPS listener
				albLsConfig := elbv2gw.ListenerConfiguration{
					ProtocolPort:       "HTTPS:443",
					DefaultCertificate: &cert,
				}
				albLbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{albLsConfig}
				hasTLS = true
			}

			// IP target type for both
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}

			// ALB Gateway listeners
			albGwListeners := []gwv1.Listener{
				{
					Name:     "http80",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
			}
			if hasTLS {
				albGwListeners = append(albGwListeners, gwv1.Listener{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
				})
			}

			// HTTPRoute for ALB
			httpr := BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, nil)

			By("deploying ALB stack", func() {
				err := albStack.DeployHTTP(ctx, nil, tf, albGwListeners, []*gwv1.HTTPRoute{httpr}, albLbcSpec, tgSpec, lrcSpec, nil, true)
				Expect(err).NotTo(HaveOccurred())
			})

			By("deploying NLB stack", func() {
				err := nlbStack.DeployFrontendNLB(ctx, albStack, tf, nlbLbcSpec, hasTLS, true)
				Expect(err).NotTo(HaveOccurred())
			})
			By("checking alb gateway status for lb dns name", func() {
				time.Sleep(2 * time.Minute)
				albDnsName = albStack.GetLoadBalancerIngressHostName()
				Expect(albDnsName).ToNot(BeEmpty())
			})
			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				albARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, albDnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(albARN).ToNot(BeEmpty())
			})
			By("checking nlb gateway status for lb dns name", func() {
				nlbDnsName = nlbStack.GetLoadBalancerIngressHostName()
				Expect(nlbDnsName).ToNot(BeEmpty())
			})
			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				nlbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, nlbDnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(nlbARN).ToNot(BeEmpty())
			})
			By("verify alb configuration", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*albStack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}

				listenerPortMap := albStack.albResourceStack.getListenersPortMap()

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, albARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internal",
					Listeners:    listenerPortMap,
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verify nlb configuration", func() {
				// No ref grants, means no tg or listener.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{}

				listenerPortMap := map[string]string{}

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, nlbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					Listeners:    listenerPortMap,
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("deploy reference grant that allows nlb <-> alb attachment", func() {
				var err error
				refGrant, err = nlbStack.CreateFENLBReferenceGrant(ctx, tf, albStack.albResourceStack.commonStack.ns)
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(2 * time.Minute)
			})
			By("validate lb composition", func() {
				// No ref grants, means no tg or listener.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: 1,
						TargetType: "alb",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "HTTP",
							Port:               "traffic-port",
							Path:               "/",
							Interval:           15,
							Timeout:            5,
							HealthyThreshold:   3,
							UnhealthyThreshold: 3,
						},
					},
				}

				if hasTLS {
					expectedTargetGroups = append(expectedTargetGroups, verifier.ExpectedTargetGroup{
						Protocol:   "TCP",
						Port:       443,
						NumTargets: 1,
						TargetType: "alb",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "HTTPS",
							Port:               "traffic-port",
							Path:               "/",
							Interval:           15,
							Timeout:            5,
							HealthyThreshold:   3,
							UnhealthyThreshold: 3,
						},
					})
				}

				fmt.Printf("%+v\n", refGrant)

				listenerPortMap := nlbStack.nlbResourceStack.getListenersPortMap()

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, nlbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					Listeners:    listenerPortMap,
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verify port 80 works", func() {
				url := fmt.Sprintf("http://%v/any-path", nlbDnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
			if hasTLS {
				By("verify port 443 works", func() {
					url := fmt.Sprintf("https://%v/any-path", nlbDnsName)
					urlOptions := http.URLOptions{
						InsecureSkipVerify: true,
					}
					err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, http.ResponseCodeMatches(200))
					Expect(err).NotTo(HaveOccurred())
				})
			}
			By("remove reference grant should remove nlb listener but keep alb listener intact", func() {
				err := tf.K8sClient.Delete(ctx, refGrant)
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(2 * time.Minute)
			})
			By("verify alb configuration", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:      "HTTP",
						Port:          80,
						NumTargets:    int(*albStack.albResourceStack.commonStack.dps[0].Spec.Replicas),
						TargetType:    "ip",
						TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
					},
				}

				listenerPortMap := albStack.albResourceStack.getListenersPortMap()

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, albARN, verifier.LoadBalancerExpectation{
					Type:         "application",
					Scheme:       "internal",
					Listeners:    listenerPortMap,
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("verify nlb configuration", func() {
				// No ref grants, means no tg or listener.
				expectedTargetGroups := []verifier.ExpectedTargetGroup{}

				listenerPortMap := map[string]string{}

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, nlbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					Listeners:    listenerPortMap,
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
