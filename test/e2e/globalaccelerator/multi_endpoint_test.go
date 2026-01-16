package globalaccelerator

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/service"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"time"
)

var _ = Describe("GlobalAccelerator with multiple endpoint types", func() {

	var (
		ctx      context.Context
		agaStack *ResourceStack
		ingStack *ingress.ResourceStack
		svcName  string
		ingName  string
		baseName string
	)
	Context("Multiple endpoint", func() {
		var (
			namespace     string
			aga           *agav1beta1.GlobalAccelerator
			svcDeployment *appsv1.Deployment
			nlbSvc        *corev1.Service
		)

		BeforeEach(func() {
			if !tf.Options.EnableAGATests {
				Skip("Skipping Global Accelerator tests (requires --enable-aga-tests)")
			}
			ctx = context.Background()
			ns, err := tf.NSManager.AllocateNamespace(ctx, "aga-multi-e2e")
			Expect(err).NotTo(HaveOccurred())
			namespace = ns.Name
			tf.Logger.Info("allocated namespace for multi-endpoint test", "namespace", namespace)
			baseName = "aga-multi-" + utils.RandomDNS1123Label(8)
			svcName = baseName + "-service"
			ingName = baseName + "-ingress"
			labels := map[string]string{"app": baseName}

			// Deploy Ingress endpoint resources first
			ingDeployment := createDeployment(baseName+"-ing", namespace, labels)
			nodeSvc := createNodePortService(baseName+"-nodesvc", namespace, labels)
			ing := createALBIngress(ingName, namespace, baseName+"-nodesvc")
			ingStack = ingress.NewResourceStack([]*appsv1.Deployment{ingDeployment}, []*corev1.Service{nodeSvc}, []*networkingv1.Ingress{ing})
			err = ingStack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())

			// Deploy Service endpoint resources in the same namespace
			svcDeployment = createDeployment(baseName+"-svc", namespace, labels)
			nlbAnnotations := createServiceAnnotations("nlb-ip", "internet-facing", tf.Options.IPFamily)
			nlbSvc = createLoadBalancerService(svcName, labels, nlbAnnotations)
			nlbSvc.Namespace = namespace
			err = tf.K8sClient.Create(ctx, svcDeployment)
			Expect(err).NotTo(HaveOccurred())
			err = tf.K8sClient.Create(ctx, nlbSvc)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if nlbSvc != nil {
				err := tf.K8sClient.Delete(ctx, nlbSvc)
				if err != nil {
					tf.Logger.Info("failed to delete service", "error", err)
				}
			}
			if svcDeployment != nil {
				err := tf.K8sClient.Delete(ctx, svcDeployment)
				if err != nil {
					tf.Logger.Info("failed to delete deployment", "error", err)
				}
			}
			if ingStack != nil {
				err := ingStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
		})
		It("Should create GlobalAccelerator with Service and Ingress endpoints", func() {
			acceleratorName := "aga-multi-" + utils.RandomDNS1123Label(6)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			gaName := "aga-" + utils.RandomDNS1123Label(8)

			tf.Logger.Info("creating GlobalAccelerator with multiple endpoints in same namespace",
				"namespace", namespace,
				"svcName", svcName,
				"ingName", ingName)

			svcEndpoint := createServiceEndpoint(svcName, 128)
			aga = createAGA(gaName, namespace, acceleratorName, agav1beta1.IPAddressTypeIPV4, &[]agav1beta1.GlobalAcceleratorListener{{
				Protocol:       &protocol,
				PortRanges:     &[]agav1beta1.PortRange{{FromPort: 80, ToPort: 80}},
				ClientAffinity: agav1beta1.ClientAffinityNone,
				EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{{
					TrafficDialPercentage: awssdk.Int32(100),
					Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
						svcEndpoint,
						{Type: agav1beta1.GlobalAcceleratorEndpointTypeIngress, Name: awssdk.String(ingName)},
					},
				}},
			}})

			By("deploying GlobalAccelerator with multiple endpoint types", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS configuration", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{{
						Protocol:       "TCP",
						PortRanges:     []PortRangeExpectation{{FromPort: 80, ToPort: 80}},
						ClientAffinity: string(types.ClientAffinityNone),
						EndpointGroups: []EndpointGroupExpectation{{
							TrafficDialPercentage: 100,
							NumEndpoints:          2,
						}},
					}},
				})
				Expect(err).NotTo(HaveOccurred())
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

	Context("Multiple endpoint cross-namespace", func() {
		var (
			agaNamespace     string
			svcNamespace     string
			gatewayNamespace string
			ingNamespace     string
			svcStack         *service.ResourceStack
			gwStack          *gateway.ALBTestStack
			gatewayName      string
			acceleratorName  string
			gaName           string
			svcRefGrantName  string
			gwRefGrantName   string
			ingRefGrantName  string
		)

		BeforeEach(func() {
			if !tf.Options.EnableAGATests || !tf.Options.EnableGatewayTests {
				Skip("Skipping Global Accelerator multi cross-namespace tests (requires --enable-aga-tests and --enable-gateway-tests)")
			}
			ctx = context.Background()

			// Create four separate namespaces: one for GA, one for Service, one for Gateway, one for Ingress
			agaNs, err := tf.NSManager.AllocateNamespace(ctx, "aga-multi-cns-ga")
			Expect(err).NotTo(HaveOccurred())
			agaNamespace = agaNs.Name

			ingNs, err := tf.NSManager.AllocateNamespace(ctx, "aga-multi-cns-ing")
			Expect(err).NotTo(HaveOccurred())
			ingNamespace = ingNs.Name

			// Set up names for resources
			baseName = "aga-multi-cns-" + utils.RandomDNS1123Label(6)
			svcName = baseName + "-service"
			gatewayName = "gateway-e2e"
			ingName = baseName + "-ingress"
			acceleratorName = "aga-multi-cns-" + utils.RandomDNS1123Label(6)
			gaName = "aga-" + utils.RandomDNS1123Label(8)
			svcRefGrantName = "aga-svc-refgrant-" + utils.RandomDNS1123Label(6)
			gwRefGrantName = "aga-gw-refgrant-" + utils.RandomDNS1123Label(6)
			ingRefGrantName = "aga-ing-refgrant-" + utils.RandomDNS1123Label(6)

			// Create service in service namespace
			labels := map[string]string{
				"app": baseName,
			}

			svcDeployment := createDeployment(baseName, "", labels)
			svcAnnotations := createServiceAnnotations("nlb-ip", "internet-facing", tf.Options.IPFamily)
			svc := createLoadBalancerService(svcName, labels, svcAnnotations)

			svcStack = service.NewResourceStack(svcDeployment, svc, nil, nil, baseName, map[string]string{})
			err = svcStack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
			svcNamespace = svcStack.GetNamespace()

			// Create Gateway in Gateway namespace
			gwStack = &gateway.ALBTestStack{}
			scheme := elbv2gw.LoadBalancerSchemeInternetFacing
			listeners := []gwv1.Listener{{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: gwv1.PortNumber(80)}}
			httpRoute := gateway.BuildHTTPRoute(nil, nil, nil)
			err = gwStack.DeployHTTP(ctx, nil, tf, listeners, []*gwv1.HTTPRoute{httpRoute}, elbv2gw.LoadBalancerConfigurationSpec{Scheme: &scheme}, elbv2gw.TargetGroupConfigurationSpec{}, elbv2gw.ListenerRuleConfigurationSpec{}, nil, false)
			Expect(err).NotTo(HaveOccurred())
			gatewayNamespace = gwStack.GetNamespace()

			// Deploy Ingress endpoint resources in Ingress namespace
			ingDeployment := createDeployment(baseName+"-ing", ingNamespace, labels)
			nodeSvc := createNodePortService(baseName+"-nodesvc", ingNamespace, labels)
			ing := createALBIngress(ingName, ingNamespace, baseName+"-nodesvc")
			ingStack = ingress.NewResourceStack([]*appsv1.Deployment{ingDeployment}, []*corev1.Service{nodeSvc}, []*networkingv1.Ingress{ing})
			err = ingStack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if svcStack != nil {
				err := svcStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if gwStack != nil {
				err := gwStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if ingStack != nil {
				err := ingStack.Cleanup(ctx, tf)
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
			if svcNamespace != "" {
				By("cleaning up Service namespace", func() {
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: svcNamespace},
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
			if ingNamespace != "" {
				By("cleaning up Ingress namespace", func() {
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: ingNamespace},
					}
					err := tf.K8sClient.Delete(ctx, ns)
					Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrors.IsNotFound)))
					err = tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
					Expect(err).NotTo(HaveOccurred())
				})
			}
		})

		It("Should create and verify GlobalAccelerator with multiple cross-namespace endpoints", func() {
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP

			// Create GA in GA namespace with cross-namespace references to Service, Gateway and Ingress
			aga := &agav1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gaName,
					Namespace: agaNamespace,
				},
				Spec: agav1beta1.GlobalAcceleratorSpec{
					Name:          awssdk.String(acceleratorName),
					IPAddressType: agav1beta1.IPAddressTypeIPV4,
					Listeners: &[]agav1beta1.GlobalAcceleratorListener{
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
										// Service endpoint from Service namespace
										{
											Type:      agav1beta1.GlobalAcceleratorEndpointTypeService,
											Name:      awssdk.String(svcName),
											Namespace: awssdk.String(svcNamespace),
											Weight:    awssdk.Int32(128),
										},
										// Gateway endpoint from Gateway namespace
										{
											Type:      agav1beta1.GlobalAcceleratorEndpointTypeGateway,
											Name:      awssdk.String(gatewayName),
											Namespace: awssdk.String(gatewayNamespace),
											Weight:    awssdk.Int32(128),
										},
										// Ingress endpoint from Ingress namespace
										{
											Type:      agav1beta1.GlobalAcceleratorEndpointTypeIngress,
											Name:      awssdk.String(ingName),
											Namespace: awssdk.String(ingNamespace),
											Weight:    awssdk.Int32(128),
										},
									},
								},
							},
						},
					},
				},
			}

			By("deploying GlobalAccelerator with multiple cross-namespace endpoints", func() {
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

			By("creating reference grant for Service in Service namespace", func() {
				err := CreateReferenceGrant(
					ctx,
					tf,
					svcRefGrantName,
					agaNamespace,
					svcNamespace,
					shared_constants.CoreAPIGroup,
					shared_constants.ServiceKind,
				)
				Expect(err).NotTo(HaveOccurred())

				// Give some time for the controller to reconcile after ReferenceGrant is created
				time.Sleep(5 * time.Second)
			})

			By("verifying one endpoint is added after first ReferenceGrant", func() {
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

			By("creating reference grant for Gateway in Gateway namespace", func() {
				err := CreateReferenceGrant(
					ctx,
					tf,
					gwRefGrantName,
					agaNamespace,
					gatewayNamespace,
					shared_constants.GatewayAPIResourcesGroup,
					shared_constants.GatewayApiKind,
				)
				Expect(err).NotTo(HaveOccurred())

				// Give some time for the controller to reconcile after ReferenceGrant is created
				time.Sleep(5 * time.Second)
			})

			By("verifying two endpoints are added after second ReferenceGrant", func() {
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
									{TrafficDialPercentage: 100, NumEndpoints: 2},
								},
							},
						},
					})
				}, utils.PollTimeoutLong, utils.PollIntervalMedium).Should(Succeed())
			})

			By("creating reference grant for Ingress in Ingress namespace", func() {
				err := CreateReferenceGrant(
					ctx,
					tf,
					ingRefGrantName,
					agaNamespace,
					ingNamespace,
					shared_constants.IngressAPIGroup,
					shared_constants.IngressKind,
				)
				Expect(err).NotTo(HaveOccurred())

				// Give some time for the controller to reconcile after ReferenceGrant is created
				time.Sleep(5 * time.Second)
			})

			By("verifying all three endpoints are added after third ReferenceGrant", func() {
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
									{TrafficDialPercentage: 100, NumEndpoints: 3},
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

			By("deleting the Service reference grant", func() {
				refGrant := &gwbeta1.ReferenceGrant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      svcRefGrantName,
						Namespace: svcNamespace,
					},
				}
				err := tf.K8sClient.Delete(ctx, refGrant)
				Expect(err).NotTo(HaveOccurred())

				// Give some time for the controller to reconcile after ReferenceGrant deletion
				time.Sleep(5 * time.Second)
			})

			By("verifying only two endpoints remain after Service ReferenceGrant deletion", func() {
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
									{TrafficDialPercentage: 100, NumEndpoints: 2},
								},
							},
						},
					})
				}, utils.PollTimeoutLong, utils.PollIntervalMedium).Should(Succeed())
			})

			By("verifying traffic still flows through GlobalAccelerator with remaining endpoints", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
