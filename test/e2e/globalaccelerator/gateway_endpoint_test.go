package globalaccelerator

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"time"
)

var _ = Describe("GlobalAccelerator with Gateway endpoint", func() {
	var (
		ctx      context.Context
		agaStack *ResourceStack
		aga      *agav1beta1.GlobalAccelerator
	)

	BeforeEach(func() {
		if !tf.Options.EnableAGATests || !tf.Options.EnableGatewayTests {
			Skip("Skipping Global Accelerator Gateway endpoint tests (requires --enable-aga-tests and --enable-gateway-tests)")
		}
		ctx = context.Background()
	})

	Context("Gateway endpoint with ALB", func() {
		var (
			gwStack     *gateway.ALBTestStack
			gatewayName string
			namespace   string
		)

		BeforeEach(func() {
			gwStack = &gateway.ALBTestStack{}
			scheme := elbv2gw.LoadBalancerSchemeInternetFacing
			listeners := []gwv1.Listener{{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: gwv1.PortNumber(80)}}
			httpRoute := gateway.BuildHTTPRoute(nil, nil, nil)
			err := gwStack.DeployHTTP(ctx, nil, tf, listeners, []*gwv1.HTTPRoute{httpRoute}, elbv2gw.LoadBalancerConfigurationSpec{Scheme: &scheme}, elbv2gw.TargetGroupConfigurationSpec{}, elbv2gw.ListenerRuleConfigurationSpec{}, nil, false)
			Expect(err).NotTo(HaveOccurred())
			namespace = gwStack.GetNamespace()
			gatewayName = "gateway-e2e"
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if gwStack != nil {
				err := gwStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should create and verify GlobalAccelerator with ALB Gateway endpoint", func() {
			acceleratorName := "aga-alb-gw-" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			aga = createAGAWithGatewayEndpoint(gaName, namespace, acceleratorName, gatewayName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{
					{
						Protocol: &protocol,
						PortRanges: &[]agav1beta1.PortRange{
							{FromPort: 80, ToPort: 80},
						},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
							{
								TrafficDialPercentage: awssdk.Int32(100),
								Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
									{
										Type: agav1beta1.GlobalAcceleratorEndpointTypeGateway,
										Name: awssdk.String(gatewayName),
									},
								},
							},
						},
					},
				})

			By("deploying GlobalAccelerator", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying AWS GlobalAccelerator configuration", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{
						{
							Protocol: string(types.ProtocolTcp),
							PortRanges: []PortRangeExpectation{
								{FromPort: 80, ToPort: 80},
							},
							ClientAffinity: string(types.ClientAffinityNone),
							EndpointGroups: []EndpointGroupExpectation{
								{NumEndpoints: 1},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Gateway endpoint with NLB", func() {
		var (
			gwStack     *gateway.NLBTestStack
			gatewayName string
			namespace   string
		)

		BeforeEach(func() {
			if tf.Options.IPFamily == framework.IPv6 {
				Skip("Skipping test for IPv6")
			}
			gwStack = &gateway.NLBTestStack{}
			scheme := elbv2gw.LoadBalancerSchemeInternetFacing
			err := gwStack.Deploy(ctx, tf, nil, elbv2gw.LoadBalancerConfigurationSpec{Scheme: &scheme}, elbv2gw.TargetGroupConfigurationSpec{}, false)
			Expect(err).NotTo(HaveOccurred())
			namespace = gwStack.GetNamespace()
			gatewayName = "gateway-e2e"
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if gwStack != nil {
				err := gwStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should create and verify GlobalAccelerator with NLB Gateway endpoint", func() {
			acceleratorName := "aga-nlb-gw-" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			aga = createAGAWithGatewayEndpoint(gaName, namespace, acceleratorName, gatewayName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{
					{
						Protocol: &protocol,
						PortRanges: &[]agav1beta1.PortRange{
							{FromPort: 80, ToPort: 80},
						},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
							{
								TrafficDialPercentage: awssdk.Int32(100),
								Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
									{
										Type: agav1beta1.GlobalAcceleratorEndpointTypeGateway,
										Name: awssdk.String(gatewayName),
									},
								},
							},
						},
					},
				})

			By("deploying GlobalAccelerator", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying AWS GlobalAccelerator configuration", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{
						{
							Protocol: string(types.ProtocolTcp),
							PortRanges: []PortRangeExpectation{
								{FromPort: 80, ToPort: 80},
							},
							ClientAffinity: string(types.ClientAffinityNone),
							EndpointGroups: []EndpointGroupExpectation{
								{NumEndpoints: 1},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Gateway endpoint with ALB with cross-namespace reference", func() {
		var (
			agaNamespace     string
			gatewayNamespace string
			gwStack          *gateway.ALBTestStack
			gatewayName      string
			acceleratorName  string
			gaName           string
			refGrantName     string
		)

		BeforeEach(func() {
			agaNs, err := tf.NSManager.AllocateNamespace(ctx, "aga-albgw-cns-ga")
			Expect(err).NotTo(HaveOccurred())
			agaNamespace = agaNs.Name

			// Set up names for resources
			gatewayName = "gateway-e2e"
			acceleratorName = "aga-albgw-crossns-" + utils.RandomDNS1123Label(6)
			gaName = "aga-" + utils.RandomDNS1123Label(8)
			refGrantName = "aga-albgw-refgrant-" + utils.RandomDNS1123Label(6)

			// Create Gateway in Gateway namespace
			gwStack = &gateway.ALBTestStack{}
			scheme := elbv2gw.LoadBalancerSchemeInternetFacing
			listeners := []gwv1.Listener{{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: gwv1.PortNumber(80)}}
			httpRoute := gateway.BuildHTTPRoute(nil, nil, nil)
			err = gwStack.DeployHTTP(ctx, nil, tf, listeners, []*gwv1.HTTPRoute{httpRoute}, elbv2gw.LoadBalancerConfigurationSpec{Scheme: &scheme}, elbv2gw.TargetGroupConfigurationSpec{}, elbv2gw.ListenerRuleConfigurationSpec{}, nil, false)
			Expect(err).NotTo(HaveOccurred())
			gatewayNamespace = gwStack.GetNamespace()
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if gwStack != nil {
				err := gwStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}

			// Clean up namespaces
			if agaNamespace != "" {
				By("cleaning up GA namespace", func() {
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: agaNamespace},
					}
					err := tf.K8sClient.Delete(ctx, ns)
					Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrors.IsNotFound)))
					err = tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
					Expect(err).NotTo(HaveOccurred())
				})
			}

			if gatewayNamespace != "" {
				By("cleaning up Gateway namespace", func() {
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: gatewayNamespace},
					}
					err := tf.K8sClient.Delete(ctx, ns)
					Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrors.IsNotFound)))
					err = tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
					Expect(err).NotTo(HaveOccurred())
				})
			}
		})

		It("Should create and verify GlobalAccelerator basic lifecycle - ALB gateway cross-namespace ref", func() {
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP

			// Create GA in GA namespace with cross-namespace reference to Gateway
			aga := createAGAWithGatewayEndpoint(gaName, agaNamespace, acceleratorName, gatewayName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{
					{
						Protocol: &protocol,
						PortRanges: &[]agav1beta1.PortRange{
							{FromPort: 80, ToPort: 80},
						},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
							{
								TrafficDialPercentage: awssdk.Int32(100),
								Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
									{
										Type:      agav1beta1.GlobalAcceleratorEndpointTypeGateway,
										Name:      awssdk.String(gatewayName),
										Namespace: awssdk.String(gatewayNamespace),
									},
								},
							},
						},
					},
				})

			By("deploying GlobalAccelerator", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS GlobalAccelerator configuration has no endpoints initially", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{
						{
							Protocol: string(types.ProtocolTcp),
							PortRanges: []PortRangeExpectation{
								{FromPort: 80, ToPort: 80},
							},
							ClientAffinity: string(types.ClientAffinityNone),
							EndpointGroups: []EndpointGroupExpectation{
								{TrafficDialPercentage: 100, NumEndpoints: 0},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating reference grant in gateway namespace", func() {
				err := CreateReferenceGrant(
					ctx,
					tf,
					refGrantName,
					agaNamespace,
					gatewayNamespace,
					shared_constants.GatewayAPIResourcesGroup,
					shared_constants.GatewayApiKind,
				)
				Expect(err).NotTo(HaveOccurred())

				// Give some time for the controller to reconcile after ReferenceGrant is created
				time.Sleep(10 * time.Second)
			})

			By("verifying endpoints are now added after ReferenceGrant", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				Eventually(func() error {
					return verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
						Name:          acceleratorName,
						IPAddressType: string(types.IpAddressTypeIpv4),
						Status:        string(types.AcceleratorStatusDeployed),
						Listeners: []ListenerExpectation{
							{
								Protocol: string(types.ProtocolTcp),
								PortRanges: []PortRangeExpectation{
									{FromPort: 80, ToPort: 80},
								},
								ClientAffinity: string(types.ClientAffinityNone),
								EndpointGroups: []EndpointGroupExpectation{
									{TrafficDialPercentage: 100, NumEndpoints: 1},
								},
							},
						},
					})
				}, utils.PollTimeoutLong, utils.PollIntervalMedium).Should(Succeed())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Gateway endpoint with NLB with cross-namespace reference", func() {
		var (
			agaNamespace     string
			gatewayNamespace string
			gwStack          *gateway.NLBTestStack
			gatewayName      string
			acceleratorName  string
			gaName           string
			refGrantName     string
		)

		BeforeEach(func() {
			if tf.Options.IPFamily == framework.IPv6 {
				Skip("Skipping test for IPv6")
			}

			agaNs, err := tf.NSManager.AllocateNamespace(ctx, "aga-nlbgw-cns-ga")
			Expect(err).NotTo(HaveOccurred())
			agaNamespace = agaNs.Name

			// Set up names for resources
			gatewayName = "gateway-e2e"
			acceleratorName = "aga-nlbgw-crossns-" + utils.RandomDNS1123Label(6)
			gaName = "aga-" + utils.RandomDNS1123Label(8)
			refGrantName = "aga-nlbgw-refgrant-" + utils.RandomDNS1123Label(6)

			// Create Gateway in Gateway namespace
			gwStack = &gateway.NLBTestStack{}
			scheme := elbv2gw.LoadBalancerSchemeInternetFacing
			err = gwStack.Deploy(ctx, tf, nil, elbv2gw.LoadBalancerConfigurationSpec{Scheme: &scheme}, elbv2gw.TargetGroupConfigurationSpec{}, false)
			Expect(err).NotTo(HaveOccurred())
			gatewayNamespace = gwStack.GetNamespace()
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if gwStack != nil {
				err := gwStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}

			// Clean up namespaces
			if agaNamespace != "" {
				By("cleaning up GA namespace", func() {
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: agaNamespace},
					}
					err := tf.K8sClient.Delete(ctx, ns)
					Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrors.IsNotFound)))
					err = tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
					Expect(err).NotTo(HaveOccurred())
				})
			}

			if gatewayNamespace != "" {
				By("cleaning up Gateway namespace", func() {
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: gatewayNamespace},
					}
					err := tf.K8sClient.Delete(ctx, ns)
					Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrors.IsNotFound)))
					err = tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
					Expect(err).NotTo(HaveOccurred())
				})
			}
		})

		It("Should create and verify GlobalAccelerator basic lifecycle - NLB gateway cross-namespace ref", func() {
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP

			// Create GA in GA namespace with cross-namespace reference to Gateway
			aga := createAGAWithGatewayEndpoint(gaName, agaNamespace, acceleratorName, gatewayName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{
					{
						Protocol: &protocol,
						PortRanges: &[]agav1beta1.PortRange{
							{FromPort: 80, ToPort: 80},
						},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
							{
								TrafficDialPercentage: awssdk.Int32(100),
								Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
									{
										Type:      agav1beta1.GlobalAcceleratorEndpointTypeGateway,
										Name:      awssdk.String(gatewayName),
										Namespace: awssdk.String(gatewayNamespace),
									},
								},
							},
						},
					},
				})

			By("deploying GlobalAccelerator", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS GlobalAccelerator configuration has no endpoints initially", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{
						{
							Protocol: string(types.ProtocolTcp),
							PortRanges: []PortRangeExpectation{
								{FromPort: 80, ToPort: 80},
							},
							ClientAffinity: string(types.ClientAffinityNone),
							EndpointGroups: []EndpointGroupExpectation{
								{TrafficDialPercentage: 100, NumEndpoints: 0},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating reference grant in gateway namespace", func() {
				err := CreateReferenceGrant(
					ctx,
					tf,
					refGrantName,
					agaNamespace,
					gatewayNamespace,
					shared_constants.GatewayAPIResourcesGroup,
					shared_constants.GatewayApiKind,
				)
				Expect(err).NotTo(HaveOccurred())

				// Give some time for the controller to reconcile after ReferenceGrant is created
				time.Sleep(10 * time.Second)
			})

			By("verifying endpoints are now added after ReferenceGrant", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				Eventually(func() error {
					return verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
						Name:          acceleratorName,
						IPAddressType: string(types.IpAddressTypeIpv4),
						Status:        string(types.AcceleratorStatusDeployed),
						Listeners: []ListenerExpectation{
							{
								Protocol: string(types.ProtocolTcp),
								PortRanges: []PortRangeExpectation{
									{FromPort: 80, ToPort: 80},
								},
								ClientAffinity: string(types.ClientAffinityNone),
								EndpointGroups: []EndpointGroupExpectation{
									{TrafficDialPercentage: 100, NumEndpoints: 1},
								},
							},
						},
					})
				}, utils.PollTimeoutLong, utils.PollIntervalMedium).Should(Succeed())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
