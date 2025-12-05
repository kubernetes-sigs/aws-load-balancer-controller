package globalaccelerator

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("GlobalAccelerator with Ingress endpoint", func() {
	var (
		ctx       context.Context
		agaStack  *ResourceStack
		ingStack  EndpointStack
		namespace string
		ingName   string
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
		ns, err := tf.NSManager.AllocateNamespace(ctx, "aga-ing-e2e")
		Expect(err).NotTo(HaveOccurred())
		namespace = ns.Name
		baseName = "aga-ing-" + utils.RandomDNS1123Label(8)
		ingName = baseName + "-ingress"
		labels := map[string]string{
			"app": baseName,
		}
		replicas := int32(2)
		dpImage := utils.GetDeploymentImage(tf.Options.TestImageRegistry, utils.HelloImage)

		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      baseName + "-deployment",
				Namespace: namespace,
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

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      baseName + "-service",
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeNodePort,
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

		pathType := networkingv1.PathTypePrefix
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingName,
				Namespace: namespace,
				Annotations: map[string]string{
					"alb.ingress.kubernetes.io/scheme": "internet-facing",
				},
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: awssdk.String("alb"),
				Rules: []networkingv1.IngressRule{
					{
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path:     "/",
										PathType: &pathType,
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: baseName + "-service",
												Port: networkingv1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		ingStack = ingress.NewResourceStack([]*appsv1.Deployment{deployment}, []*corev1.Service{svc}, []*networkingv1.Ingress{ing})
		err = ingStack.Deploy(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if agaStack != nil {
			By("cleanup GlobalAccelerator stack", func() {
				err := agaStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				agaStack = nil
			})
		}
		if ingStack != nil {
			By("cleanup ingress stack", func() {
				err := ingStack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
				ingStack = nil
			})
		}
	})

	Context("Basic Ingress endpoint with configuration verification", func() {
		It("Should create and verify GlobalAccelerator basic lifecycle", func() {
			acceleratorName := "aga-basic-" + utils.RandomDNS1123Label(6)
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
							Protocol: string(types.ProtocolTcp),
							PortRanges: []PortRangeExpectation{
								{FromPort: 80, ToPort: 80},
								{FromPort: 443, ToPort: 443},
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
					},
				},
			}

			By("deploying GlobalAccelerator without protocol and port ranges", func() {
				agaStack = NewResourceStack(nil, aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying controller auto-discovered protocol and ports from Ingress", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				Expect(gaARN).NotTo(BeEmpty())
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
