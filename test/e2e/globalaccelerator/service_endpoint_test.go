package globalaccelerator

import (
	"context"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/service"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("GlobalAccelerator with Service endpoint", func() {
	var (
		ctx       context.Context
		agaStack  *ResourceStack
		svcStack  EndpointStack
		namespace string
		svcName   string
		aga       *agav1beta1.GlobalAccelerator
		baseName  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		if tf.Options.ControllerImage != "" {
			By("upgrade controller with GlobalAccelerator enabled", func() {
				err := tf.CTRLInstallationManager.UpgradeController(tf.Options.ControllerImage, false, false, true)
				Expect(err).NotTo(HaveOccurred())
				tf.Logger.Info("Controller upgrade completed, waiting for rollout")
				time.Sleep(60 * time.Second)
				tf.Logger.Info("Controller should be ready now")
			})
		}
		baseName = "aga-svc-" + utils.RandomDNS1123Label(8)
		svcName = baseName + "-service"
		labels := map[string]string{
			"app": baseName,
		}
		replicas := int32(2)
		dpImage := utils.GetDeploymentImage(tf.Options.TestImageRegistry, utils.HelloImage)

		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: baseName + "-deployment",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "app",
								Image:           dpImage,
								ImagePullPolicy: corev1.PullAlways,
								Ports: []corev1.ContainerPort{
									{ContainerPort: 80},
								},
							},
						},
					},
				},
			},
		}

		annotation := map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type":   "nlb-ip",
			"service.beta.kubernetes.io/aws-load-balancer-scheme": "internet-facing",
		}
		if tf.Options.IPFamily == framework.IPv6 {
			annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
		}

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcName,
				Annotations: annotation,
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeLoadBalancer,
				Selector: labels,
				Ports: []corev1.ServicePort{
					{
						Port:       80,
						TargetPort: intstr.FromInt(80),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		}

		svcStack = service.NewResourceStack(deployment, svc, nil, baseName, false)
		err := svcStack.Deploy(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
		namespace = svcStack.GetNamespace()
	})

	AfterEach(func() {
		if agaStack != nil {
			By("cleanup GlobalAccelerator stack", func() {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				agaStack = nil
			})
		}
		if svcStack != nil {
			By("cleanup service stack", func() {
				err := svcStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				svcStack = nil
			})
		}
	})

	Context("Service endpoint with IP target type", func() {
		It("Should create and verify GlobalAccelerator lifecycle", func() {
			acceleratorName := "aga-lifecycle-" + utils.RandomDNS1123Label(6)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			aga = &agav1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gaName,
					Namespace: namespace,
				},
				Spec: agav1beta1.GlobalAcceleratorSpec{
					Name:          &acceleratorName,
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
										{
											Type:   agav1beta1.GlobalAcceleratorEndpointTypeService,
											Name:   awssdk.String(svcName),
											Weight: awssdk.Int32(128),
										},
									},
								},
							},
						},
					},
				},
			}

			By("deploying GlobalAccelerator", func() {
				agaStack = NewResourceStack(nil, aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				Expect(gaARN).NotTo(BeEmpty())
				dnsName := agaStack.GetGlobalAcceleratorDNSName()
				Expect(dnsName).NotTo(BeEmpty())
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
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
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
							ClientAffinity: string(types.ClientAffinitySourceIp),
							EndpointGroups: []EndpointGroupExpectation{
								{TrafficDialPercentage: 50},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying traffic still flows after update", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Service endpoint with Instance target type", func() {
		var (
			instanceSvcStack EndpointStack
			instanceSvcName  string
			instanceNs       string
		)

		BeforeEach(func() {
			instanceBaseName := "aga-inst-" + utils.RandomDNS1123Label(8)
			instanceSvcName = instanceBaseName + "-service"
			labels := map[string]string{
				"app": instanceBaseName,
			}
			replicas := int32(2)
			dpImage := utils.GetDeploymentImage(tf.Options.TestImageRegistry, utils.HelloImage)

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: instanceBaseName + "-deployment",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: labels,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:            "app",
									Image:           dpImage,
									ImagePullPolicy: corev1.PullAlways,
									Ports: []corev1.ContainerPort{
										{ContainerPort: 80},
									},
								},
							},
						},
					},
				},
			}

			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
				"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
				"service.beta.kubernetes.io/aws-load-balancer-scheme":          "internet-facing",
			}
			if tf.Options.IPFamily == framework.IPv6 {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
			}

			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        instanceSvcName,
					Annotations: annotation,
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: labels,
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			}

			instanceSvcStack = service.NewResourceStack(deployment, svc, nil, instanceBaseName, false)
			err := instanceSvcStack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
			instanceNs = instanceSvcStack.GetNamespace()
		})

		AfterEach(func() {
			if agaStack != nil {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				agaStack = nil
			}
			if instanceSvcStack != nil {
				err := instanceSvcStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				instanceSvcStack = nil
			}
		})

		It("Should update GlobalAccelerator endpoint when load balancer scheme changes", func() {
			acceleratorName := "aga-inst-" + utils.RandomDNS1123Label(6)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			gaName := "aga-" + utils.RandomDNS1123Label(8)
			aga = &agav1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gaName,
					Namespace: instanceNs,
				},
				Spec: agav1beta1.GlobalAcceleratorSpec{
					Name:          &acceleratorName,
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
										{
											Type:   agav1beta1.GlobalAcceleratorEndpointTypeService,
											Name:   awssdk.String(instanceSvcName),
											Weight: awssdk.Int32(128),
										},
									},
								},
							},
						},
					},
				},
			}

			By("deploying GlobalAccelerator", func() {
				agaStack = NewResourceStack(nil, aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				Expect(gaARN).NotTo(BeEmpty())
				dnsName := agaStack.GetGlobalAcceleratorDNSName()
				Expect(dnsName).NotTo(BeEmpty())
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
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})

			var originalLBHostname string
			By("capturing original load balancer hostname", func() {
				svc := &corev1.Service{}
				err := tf.K8sClient.Get(ctx, k8s.NamespacedName(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instanceSvcName,
						Namespace: instanceNs,
					},
				}), svc)
				Expect(err).NotTo(HaveOccurred())
				Expect(svc.Status.LoadBalancer.Ingress).NotTo(BeEmpty())
				originalLBHostname = svc.Status.LoadBalancer.Ingress[0].Hostname
				Expect(originalLBHostname).NotTo(BeEmpty())
			})

			By("updating service scheme to internal", func() {
				svc := &corev1.Service{}
				err := tf.K8sClient.Get(ctx, k8s.NamespacedName(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instanceSvcName,
						Namespace: instanceNs,
					},
				}), svc)
				Expect(err).NotTo(HaveOccurred())
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"] = "internal"
				err = tf.K8sClient.Update(ctx, svc)
				Expect(err).NotTo(HaveOccurred())

				err = tf.K8sClient.Get(ctx, k8s.NamespacedName(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instanceSvcName,
						Namespace: instanceNs,
					},
				}), svc)
				Expect(err).NotTo(HaveOccurred())
				Expect(svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"]).To(Equal("internal"))
			})

			var newLBHostname string
			By("verifying load balancer was replaced with internal scheme", func() {
				Eventually(func() bool {
					svc := &corev1.Service{}
					err := tf.K8sClient.Get(ctx, k8s.NamespacedName(&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      instanceSvcName,
							Namespace: instanceNs,
						},
					}), svc)
					if err != nil || len(svc.Status.LoadBalancer.Ingress) == 0 {
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
	})
})
