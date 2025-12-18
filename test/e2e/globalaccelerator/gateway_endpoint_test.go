package globalaccelerator

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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
})
