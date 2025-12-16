package globalaccelerator

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("GlobalAccelerator with Ingress endpoint", func() {
	var (
		ctx       context.Context
		agaStack  *ResourceStack
		ingStack  *ingress.ResourceStack
		namespace string
		ingName   string
		aga       *agav1beta1.GlobalAccelerator
		baseName  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		if !tf.Options.EnableAGATests {
			Skip("Skipping Global Accelerator tests (requires --enable-aga-tests)")
		}
		ns, err := tf.NSManager.AllocateNamespace(ctx, "aga-ing-e2e")
		Expect(err).NotTo(HaveOccurred())
		namespace = ns.Name
		baseName = "aga-ing-" + utils.RandomDNS1123Label(8)
		ingName = baseName + "-ingress"
		labels := map[string]string{
			"app": baseName,
		}

		deployment := createDeployment(baseName, namespace, labels)
		svc := createNodePortService(baseName+"-service", namespace, labels)
		ing := createALBIngress(ingName, namespace, baseName+"-service")

		ingStack = ingress.NewResourceStack([]*appsv1.Deployment{deployment}, []*corev1.Service{svc}, []*networkingv1.Ingress{ing})
		err = ingStack.Deploy(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if agaStack != nil {
			err := agaStack.Cleanup(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		}
		if ingStack != nil {
			err := ingStack.Cleanup(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		}
		if namespace != "" {
			By("teardown namespace", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: namespace},
				}
				tf.Logger.Info("deleting namespace", "name", namespace)
				err := tf.K8sClient.Delete(ctx, ns)
				Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrors.IsNotFound)))
				tf.Logger.Info("deleted namespace", "name", namespace)
				tf.Logger.Info("waiting namespace becomes deleted", "name", namespace)
				err = tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
				Expect(err).NotTo(HaveOccurred())
				tf.Logger.Info("namespace becomes deleted", "name", namespace)
			})
		}
	})

	Context("Basic Ingress endpoint with configuration verification", func() {
		It("Should create and verify GlobalAccelerator basic lifecycle", func() {
			acceleratorName := "aga-ing-basic-" + utils.RandomDNS1123Label(6)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			aga = createAGAWithIngressEndpoint(gaName, namespace, acceleratorName, ingName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{
					{
						Protocol: &protocol,
						PortRanges: &[]agav1beta1.PortRange{
							{FromPort: 80, ToPort: 80},
							{FromPort: 443, ToPort: 443},
						},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
							{
								TrafficDialPercentage: awssdk.Int32(100),
								Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
									{
										Type: agav1beta1.GlobalAcceleratorEndpointTypeIngress,
										Name: awssdk.String(ingName),
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
								{FromPort: 443, ToPort: 443},
							},
							ClientAffinity: string(types.ClientAffinityNone),
							EndpointGroups: []EndpointGroupExpectation{
								{TrafficDialPercentage: 100, NumEndpoints: 1},
							},
						},
					},
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

			By("updating GlobalAccelerator port ranges", func() {
				err := agaStack.UpdateGlobalAccelerator(ctx, tf, func(aga *agav1beta1.GlobalAccelerator) {
					(*aga.Spec.Listeners)[0].PortRanges = &[]agav1beta1.PortRange{
						{FromPort: 80, ToPort: 80},
					}
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying updated AWS configuration", func() {
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

			By("verifying traffic still flows after update", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Auto-discovery with Ingress endpoint", func() {
		It("Should auto-discover protocol and ports from Ingress", func() {
			acceleratorName := "aga-autodiscovery-" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			aga = createAGAWithIngressEndpoint(gaName, namespace, acceleratorName, ingName, "", nil)

			By("deploying GlobalAccelerator without protocol and port ranges", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying controller auto-discovered protocol and ports from Ingress and applied CRD defaults", func() {
				verifyAGAStatusFields(agaStack)
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
								{NumEndpoints: 1, TrafficDialPercentage: 100},
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

	Context("IPV4 to DUAL_STACK migration", func() {
		It("Should migrate from IPV4 to DUAL_STACK address type", func() {
			if tf.Options.IPFamily != framework.IPv6 {
				Skip("Test requires IPv6 cluster")
			}
			acceleratorName := "aga-migration-" + utils.RandomDNS1123Label(6)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			aga = createAGAWithIngressEndpoint(gaName, namespace, acceleratorName, ingName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{
					{
						Protocol:       &protocol,
						PortRanges:     &[]agav1beta1.PortRange{{FromPort: 80, ToPort: 80}},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{
							{
								TrafficDialPercentage: awssdk.Int32(100),
								Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
									{
										Type: agav1beta1.GlobalAcceleratorEndpointTypeIngress,
										Name: awssdk.String(ingName),
									},
								},
							},
						},
					},
				})

			By("deploying GlobalAccelerator with IPV4", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS GlobalAccelerator IPV4 configuration", func() {
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
								{TrafficDialPercentage: 100, NumEndpoints: 1},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying IPV4 status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying IPV4 traffic flows", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})

			By("updating to DUAL_STACK address type", func() {
				err := agaStack.UpdateGlobalAccelerator(ctx, tf, func(aga *agav1beta1.GlobalAccelerator) {
					aga.Spec.IPAddressType = agav1beta1.IPAddressTypeDualStack
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS GlobalAccelerator DUAL_STACK configuration", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				Eventually(func() error {
					return verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
						Name:          acceleratorName,
						IPAddressType: string(types.IpAddressTypeDualStack),
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
				}, utils.PollTimeoutMedium, utils.PollIntervalMedium).Should(Succeed())
			})

			By("verifying DUAL_STACK status fields", func() {
				Eventually(func() string {
					_ = agaStack.RefreshGlobalAcceleratorStatus(ctx, tf)
					return agaStack.GetGlobalAcceleratorDualStackDNSName()
				}, utils.PollTimeoutMedium, utils.PollIntervalMedium).ShouldNot(BeEmpty())
			})

			By("verifying DUAL_STACK traffic flows", func() {
				err := verifyAGATrafficFlowsViaDualStack(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Port overrides with Ingress endpoint", func() {
		It("Should configure port overrides correctly", func() {
			acceleratorName := "aga-portoverride-" + utils.RandomDNS1123Label(6)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			gaName := "aga-" + utils.RandomDNS1123Label(8)

			aga = createAGAWithIngressEndpoint(gaName, namespace, acceleratorName, ingName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{{
					Protocol:       &protocol,
					PortRanges:     &[]agav1beta1.PortRange{{FromPort: 443, ToPort: 443}},
					ClientAffinity: agav1beta1.ClientAffinityNone,
					EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{{
						TrafficDialPercentage: awssdk.Int32(100),
						PortOverrides: &[]agav1beta1.PortOverride{
							{ListenerPort: 443, EndpointPort: 80},
						},
						Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
							{Type: agav1beta1.GlobalAcceleratorEndpointTypeIngress, Name: awssdk.String(ingName)},
						},
					}},
				}})

			By("deploying GlobalAccelerator with port overrides", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying port overrides configuration", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{{
						Protocol:       "TCP",
						PortRanges:     []PortRangeExpectation{{FromPort: 443, ToPort: 443}},
						ClientAffinity: string(types.ClientAffinityNone),
						EndpointGroups: []EndpointGroupExpectation{{
							TrafficDialPercentage: 100,
							NumEndpoints:          1,
							PortOverrides: []PortOverrideExpectation{
								{ListenerPort: 443, EndpointPort: 80},
							},
						}},
					}},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through port override", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack, 443)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
