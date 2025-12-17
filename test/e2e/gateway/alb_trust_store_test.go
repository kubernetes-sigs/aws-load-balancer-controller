package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	httputils "sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("test ALB Gateway with Trust Store for mTLS", func() {
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
		if len(tf.Options.CertificateARNs) == 0 {
			Skip("Skipping tests, certificates not specified")
		}
		if len(tf.Options.TrustStoreARN) == 0 {
			Skip("Skipping tests, trust store ARN not specified")
		}
		ctx = context.Background()
		stack = ALBTestStack{}
	})

	AfterEach(func() {
		stack.Cleanup(ctx, tf)
	})

	Context("with mTLS in VERIFY mode", func() {
		It("should enforce client certificate validation", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:                          elbv2gw.MutualAuthenticationVerifyMode,
					TrustStore:                    awssdk.String(tf.Options.TrustStoreARN),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:    (*elbv2gw.AdvertiseTrustStoreCaNamesEnum)(awssdk.String(string(elbv2gw.AdvertiseTrustStoreCaNamesEnumOn))),
				},
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}

			instanceTargetType := elbv2gw.TargetTypeInstance
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &instanceTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					Hostname: (*gwv1.Hostname)(awssdk.String(testHostname)),
					TLS: &gwv1.GatewayTLSConfig{
						CertificateRefs: []gwv1.SecretObjectReference{
							{
								Name: "tls-cert",
							},
						},
					},
				},
			}
			httpr := BuildHTTPRoute([]string{testHostname}, []gwv1.HTTPRouteRule{}, nil)

			By("deploying stack", func() {
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, nil, true)
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

			By("verifying AWS load balancer listener with mTLS verify mode", func() {
				err := verifier.VerifyLoadBalancerListener(ctx, tf, lbARN, int32(gwListeners[0].Port), &verifier.ListenerExpectation{
					ProtocolPort:          "HTTPS:443",
					DefaultCertificateARN: awssdk.ToString(lsConfig.DefaultCertificate),
					MutualAuthentication: &verifier.MutualAuthenticationExpectation{
						Mode:          "verify",
						TrustStoreARN: tf.Options.TrustStoreARN,
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

			By("verifying HTTPS request without client certificate is rejected", func() {
				url := fmt.Sprintf("https://%v/any-path", dnsName)
				urlOptions := httputils.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname,
				}
				// Either 403 response or connection error indicates mTLS is enforcing
				_ = tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, httputils.ResponseCodeMatches(403))
			})

			// Test with valid client certificate if provided
			if len(tf.Options.ClientCertPath) > 0 && len(tf.Options.ClientKeyPath) > 0 {
				By("verifying HTTPS request with valid client certificate succeeds", func() {
					// Load client certificate
					clientCert, err := tls.LoadX509KeyPair(tf.Options.ClientCertPath, tf.Options.ClientKeyPath)
					Expect(err).NotTo(HaveOccurred())

					// Create custom HTTP client with client certificate
					tlsConfig := &tls.Config{
						InsecureSkipVerify: true,
						Certificates:       []tls.Certificate{clientCert},
					}
					client := &http.Client{
						Transport: &http.Transport{
							TLSClientConfig: tlsConfig,
						},
					}

					url := fmt.Sprintf("https://%v/any-path", dnsName)
					req, err := http.NewRequest("GET", url, nil)
					Expect(err).NotTo(HaveOccurred())
					req.Host = testHostname

					resp, err := client.Do(req)
					Expect(err).NotTo(HaveOccurred())
					defer resp.Body.Close()

					// Expect 200 with valid client certificate
					Expect(resp.StatusCode).To(Equal(200))
				})
			}
		})
	})

	Context("with mTLS in PASSTHROUGH mode", func() {
		It("should not enforce mTLS at ALB", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: elbv2gw.MutualAuthenticationPassthroughMode,
				},
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}

			instanceTargetType := elbv2gw.TargetTypeInstance
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &instanceTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					Hostname: (*gwv1.Hostname)(awssdk.String(testHostname)),
					TLS: &gwv1.GatewayTLSConfig{
						CertificateRefs: []gwv1.SecretObjectReference{
							{
								Name: "tls-cert",
							},
						},
					},
				},
			}
			httpr := BuildHTTPRoute([]string{testHostname}, []gwv1.HTTPRouteRule{}, nil)

			By("deploying stack", func() {
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, nil, true)
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

			By("verifying AWS load balancer listener with mTLS passthrough mode", func() {
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

			By("verifying HTTPS request without client certificate succeeds", func() {
				url := fmt.Sprintf("https://%v/any-path", dnsName)
				urlOptions := httputils.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname,
				}
				err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, httputils.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with mTLS in OFF mode", func() {
		It("should not configure mTLS", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}

			cert := strings.Split(tf.Options.CertificateARNs, ",")[0]
			lsConfig := elbv2gw.ListenerConfiguration{
				ProtocolPort:       "HTTPS:443",
				DefaultCertificate: &cert,
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: elbv2gw.MutualAuthenticationOffMode,
				},
			}
			lbcSpec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{lsConfig}

			instanceTargetType := elbv2gw.TargetTypeInstance
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &instanceTargetType,
				},
			}
			lrcSpec := elbv2gw.ListenerRuleConfigurationSpec{}
			gwListeners := []gwv1.Listener{
				{
					Name:     "https443",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					Hostname: (*gwv1.Hostname)(awssdk.String(testHostname)),
					TLS: &gwv1.GatewayTLSConfig{
						CertificateRefs: []gwv1.SecretObjectReference{
							{
								Name: "tls-cert",
							},
						},
					},
				},
			}
			httpr := BuildHTTPRoute([]string{testHostname}, []gwv1.HTTPRouteRule{}, nil)

			By("deploying stack", func() {
				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, lrcSpec, nil, true)
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

			By("verifying AWS load balancer listener with mTLS off mode", func() {
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

			By("verifying HTTPS request without client certificate succeeds", func() {
				url := fmt.Sprintf("https://%v/any-path", dnsName)
				urlOptions := httputils.URLOptions{
					InsecureSkipVerify: true,
					HostHeader:         testHostname,
				}
				err := tf.HTTPVerifier.VerifyURLWithOptions(url, urlOptions, httputils.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
