package globalaccelerator

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/service"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("GlobalAccelerator with Service endpoint", func() {
	var (
		ctx      context.Context
		agaStack *ResourceStack
		aga      *agav1beta1.GlobalAccelerator
	)

	BeforeEach(func() {
		if !tf.Options.EnableAGATests {
			Skip("Skipping Global Accelerator tests (requires --enable-aga-tests)")
		}
		ctx = context.Background()
	})

	Context("Service endpoint with IP target type", func() {
		var (
			svcStack  *service.ResourceStack
			namespace string
			svcName   string
		)

		BeforeEach(func() {
			baseName := "aga-svc-" + utils.RandomDNS1123Label(8)
			svcName = baseName + "-service"
			labels := map[string]string{
				"app": baseName,
			}

			deployment := createDeployment(baseName, "", labels)
			annotations := createServiceAnnotations("nlb-ip", "internet-facing", tf.Options.IPFamily)
			svc := createLoadBalancerService(svcName, labels, annotations)

			svcStack = service.NewResourceStack(deployment, svc, nil, nil, baseName, map[string]string{})
			err := svcStack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
			namespace = svcStack.GetNamespace()
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
		})
		It("Should create and verify GlobalAccelerator lifecycle", func() {
			acceleratorName := "aga-svc-ip-" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			aga = createAGAWithServiceEndpoint(gaName, namespace, acceleratorName, svcName, agav1beta1.IPAddressTypeIPV4,
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
									createServiceEndpoint(svcName, 128),
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
							Protocol: "TCP",
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

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})

			By("updating GlobalAccelerator configuration", func() {
				err := agaStack.UpdateGlobalAccelerator(ctx, tf, func(ga *agav1beta1.GlobalAccelerator) {
					(*ga.Spec.Listeners)[0].ClientAffinity = agav1beta1.ClientAffinitySourceIP
					(*(*ga.Spec.Listeners)[0].EndpointGroups)[0].TrafficDialPercentage = awssdk.Int32(50)
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
								Protocol: "TCP",
								PortRanges: []PortRangeExpectation{
									{FromPort: 80, ToPort: 80},
								},
								ClientAffinity: string(types.ClientAffinitySourceIp),
								EndpointGroups: []EndpointGroupExpectation{
									{NumEndpoints: 1, TrafficDialPercentage: 50},
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

	Context("Service endpoint with Instance target type", func() {
		var (
			instanceSvcStack *service.ResourceStack
			instanceSvcName  string
			instanceNs       string
		)

		BeforeEach(func() {
			instanceBaseName := "aga-inst-" + utils.RandomDNS1123Label(8)
			instanceSvcName = instanceBaseName + "-service"
			labels := map[string]string{
				"app": instanceBaseName,
			}

			deployment := createDeployment(instanceBaseName, "", labels)
			annotations := createServiceAnnotations("external", "internet-facing", tf.Options.IPFamily)
			annotations["service.beta.kubernetes.io/aws-load-balancer-nlb-target-type"] = "instance"
			svc := createLoadBalancerService(instanceSvcName, labels, annotations)

			instanceSvcStack = service.NewResourceStack(deployment, svc, nil, nil, instanceBaseName, map[string]string{})
			err := instanceSvcStack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
			instanceNs = instanceSvcStack.GetNamespace()
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if instanceSvcStack != nil {
				err := instanceSvcStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should update GlobalAccelerator endpoint when load balancer scheme changes", func() {
			if tf.Options.IPFamily == framework.IPv6 {
				Skip("Skipping test for IPv6")
			}
			acceleratorName := "aga-svc-ins" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			aga = createAGAWithServiceEndpoint(gaName, instanceNs, acceleratorName, instanceSvcName, agav1beta1.IPAddressTypeIPV4,
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
										Type:   agav1beta1.GlobalAcceleratorEndpointTypeService,
										Name:   awssdk.String(instanceSvcName),
										Weight: awssdk.Int32(128),
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
							Protocol: "TCP",
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

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})

			var originalLBHostname string
			By("capturing original load balancer hostname", func() {
				svc := &corev1.Service{}
				svcKey := k8s.NamespacedName(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instanceSvcName,
						Namespace: instanceNs,
					},
				})
				err := tf.K8sClient.Get(ctx, svcKey, svc)
				Expect(err).NotTo(HaveOccurred())
				Expect(svc.Status.LoadBalancer.Ingress).NotTo(BeEmpty())
				originalLBHostname = svc.Status.LoadBalancer.Ingress[0].Hostname
				Expect(originalLBHostname).NotTo(BeEmpty())
			})

			By("updating service scheme to internal", func() {
				svc := &corev1.Service{}
				svcKey := k8s.NamespacedName(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instanceSvcName,
						Namespace: instanceNs,
					},
				})
				err := tf.K8sClient.Get(ctx, svcKey, svc)
				Expect(err).NotTo(HaveOccurred())
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"] = "internal"
				err = tf.K8sClient.Update(ctx, svc)
				Expect(err).NotTo(HaveOccurred())
			})

			var newLBHostname string
			By("verifying load balancer was replaced with internal scheme", func() {
				svcKey := k8s.NamespacedName(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instanceSvcName,
						Namespace: instanceNs,
					},
				})
				Eventually(func() bool {
					svc := &corev1.Service{}
					if err := tf.K8sClient.Get(ctx, svcKey, svc); err != nil {
						return false
					}
					if len(svc.Status.LoadBalancer.Ingress) == 0 {
						return false
					}
					newLBHostname = svc.Status.LoadBalancer.Ingress[0].Hostname
					return newLBHostname != "" && newLBHostname != originalLBHostname
				}, utils.PollTimeoutLong, utils.PollIntervalMedium).Should(BeTrue())

				err := verifyLoadBalancerScheme(ctx, tf, newLBHostname, "internal")
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator updated to new load balancer", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				Eventually(func() error {
					return verifyEndpointPointsToLoadBalancer(ctx, tf, gaARN, newLBHostname)
				}, utils.PollTimeoutLong*2, utils.PollIntervalMedium).Should(Succeed())
			})
		})

		It("Should create GlobalAccelerator with direct endpoint ID", func() {
			acceleratorName := "aga-endpointid-" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP

			var lbARN string
			By("getting load balancer ARN", func() {
				svc := &corev1.Service{}
				svcKey := k8s.NamespacedName(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instanceSvcName,
						Namespace: instanceNs,
					},
				})
				err := tf.K8sClient.Get(ctx, svcKey, svc)
				Expect(err).NotTo(HaveOccurred())
				Expect(svc.Status.LoadBalancer.Ingress).NotTo(BeEmpty())
				lbHostname := svc.Status.LoadBalancer.Ingress[0].Hostname
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, lbHostname)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).NotTo(BeEmpty())
			})

			aga = createAGAWithEndpointID(gaName, instanceNs, acceleratorName, lbARN, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{{
					Protocol:       &protocol,
					PortRanges:     &[]agav1beta1.PortRange{{FromPort: 80, ToPort: 80}},
					ClientAffinity: agav1beta1.ClientAffinityNone,
					EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{{
						TrafficDialPercentage: awssdk.Int32(100),
						Endpoints:             &[]agav1beta1.GlobalAcceleratorEndpoint{createEndpointIDEndpoint(lbARN)},
					}},
				}})

			By("deploying GlobalAccelerator with endpoint ID", func() {
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
					Listeners: []ListenerExpectation{{
						Protocol:       "TCP",
						PortRanges:     []PortRangeExpectation{{FromPort: 80, ToPort: 80}},
						ClientAffinity: string(types.ClientAffinityNone),
						EndpointGroups: []EndpointGroupExpectation{{NumEndpoints: 1, TrafficDialPercentage: 100}},
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

	Context("Service endpoint with instance target type and multiple listeners", func() {
		var (
			instanceSvcStack *service.ResourceStack
			instanceSvcName  string
			instanceNs       string
		)

		BeforeEach(func() {
			instanceBaseName := "aga-inst-" + utils.RandomDNS1123Label(8)
			instanceSvcName = instanceBaseName + "-service"
			labels := map[string]string{
				"app": instanceBaseName,
			}

			deployment := createDeployment(instanceBaseName, "", labels)
			annotations := createServiceAnnotations("external", "internet-facing", tf.Options.IPFamily)
			annotations["service.beta.kubernetes.io/aws-load-balancer-nlb-target-type"] = "instance"
			svc := createLoadBalancerServiceWithPorts(instanceSvcName, labels, annotations, 80, 443)

			instanceSvcStack = service.NewResourceStack(deployment, svc, nil, nil, instanceBaseName, map[string]string{})
			err := instanceSvcStack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
			instanceNs = instanceSvcStack.GetNamespace()
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
			if instanceSvcStack != nil {
				err := instanceSvcStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			}
		})
		It("Should create GlobalAccelerator with two listeners on ports 80 and 443", func() {
			acceleratorName := "aga-multi-listener-" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP

			aga = createAGAWithServiceEndpoint(gaName, instanceNs, acceleratorName, instanceSvcName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{
					{
						Protocol:       &protocol,
						PortRanges:     &[]agav1beta1.PortRange{{FromPort: 80, ToPort: 80}},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{{
							TrafficDialPercentage: awssdk.Int32(100),
							Endpoints:             &[]agav1beta1.GlobalAcceleratorEndpoint{createServiceEndpoint(instanceSvcName, 128)},
						}},
					},
					{
						Protocol:       &protocol,
						PortRanges:     &[]agav1beta1.PortRange{{FromPort: 443, ToPort: 443}},
						ClientAffinity: agav1beta1.ClientAffinityNone,
						EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{{
							TrafficDialPercentage: awssdk.Int32(100),
							Endpoints:             &[]agav1beta1.GlobalAcceleratorEndpoint{createServiceEndpoint(instanceSvcName, 128)},
						}},
					},
				})

			By("deploying GlobalAccelerator with two listeners", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS GlobalAccelerator configuration with two listeners", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{
						{
							Protocol:       "TCP",
							PortRanges:     []PortRangeExpectation{{FromPort: 80, ToPort: 80}},
							ClientAffinity: string(types.ClientAffinityNone),
							EndpointGroups: []EndpointGroupExpectation{{NumEndpoints: 1, TrafficDialPercentage: 100}},
						},
						{
							Protocol:       "TCP",
							PortRanges:     []PortRangeExpectation{{FromPort: 443, ToPort: 443}},
							ClientAffinity: string(types.ClientAffinityNone),
							EndpointGroups: []EndpointGroupExpectation{{NumEndpoints: 1, TrafficDialPercentage: 100}},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through both listeners", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack, 80, 443)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("Should create GlobalAccelerator with port range 80-443", func() {
			acceleratorName := "aga-port-range-" + utils.RandomDNS1123Label(6)
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP

			aga = createAGAWithServiceEndpoint(gaName, instanceNs, acceleratorName, instanceSvcName, agav1beta1.IPAddressTypeIPV4,
				&[]agav1beta1.GlobalAcceleratorListener{{
					Protocol:       &protocol,
					PortRanges:     &[]agav1beta1.PortRange{{FromPort: 80, ToPort: 443}},
					ClientAffinity: agav1beta1.ClientAffinityNone,
					EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{{
						TrafficDialPercentage: awssdk.Int32(100),
						Endpoints:             &[]agav1beta1.GlobalAcceleratorEndpoint{createServiceEndpoint(instanceSvcName, 128)},
					}},
				}})

			By("deploying GlobalAccelerator with port range", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS GlobalAccelerator configuration with port range", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{{
						Protocol:       "TCP",
						PortRanges:     []PortRangeExpectation{{FromPort: 80, ToPort: 443}},
						ClientAffinity: string(types.ClientAffinityNone),
						EndpointGroups: []EndpointGroupExpectation{{NumEndpoints: 1, TrafficDialPercentage: 100}},
					}},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through port range", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack, 80, 443)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})
