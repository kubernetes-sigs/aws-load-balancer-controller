package service

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
)

var _ = Describe("NLB QUIC support", func() {
	var (
		ctx         context.Context
		deployment  *appsv1.Deployment
		numReplicas int32
		name        string
		dnsName     string
		lbARN       string
		labels      map[string]string
		stack       NLBIPTestStack
	)

	BeforeEach(func() {
		ctx = context.Background()
		numReplicas = 2
		stack = NLBIPTestStack{}
		name = "quic-e2e"
		labels = map[string]string{
			"app.kubernetes.io/name":     "quic-test",
			"app.kubernetes.io/instance": name,
		}
		dpImage := utils.GetDeploymentImage(tf.Options.TestImageRegistry, utils.HelloImage)
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &numReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers": "app",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "app",
								ImagePullPolicy: corev1.PullAlways,
								Image:           dpImage,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 80,
									},
								},
							},
						},
					},
				},
			},
		}
	})

	AfterEach(func() {
		err := stack.Cleanup(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("NLB with QUIC protocol", func() {
		var svc *corev1.Service

		BeforeEach(func() {
			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":               "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-scheme":             "internet-facing",
				"service.beta.kubernetes.io/aws-load-balancer-quic-enabled-ports": "80",
			}
			svc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Annotations: annotation,
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: labels,
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolUDP,
						},
					},
				},
			}
		})

		It("Should create NLB with QUIC protocol and targets with server IDs", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			By("checking service status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})

			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})

			By("verifying load balancer has QUIC protocol", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "QUIC",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
					},
				}

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80": "QUIC",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying targets have QUIC server IDs", func() {
				targetGroups, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(targetGroups)).To(Equal(1))

				tgARN := awssdk.ToString(targetGroups[0].TargetGroupArn)
				err = verifier.VerifyTargetsHaveQUICServerIDs(ctx, tf, tgARN, int(numReplicas))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("NLB with TCP_QUIC protocol", func() {
		var svc *corev1.Service

		BeforeEach(func() {
			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":               "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-scheme":             "internet-facing",
				"service.beta.kubernetes.io/aws-load-balancer-quic-enabled-ports": "80",
			}
			svc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Annotations: annotation,
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: labels,
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   "TCP_UDP",
						},
					},
				},
			}
		})

		It("Should create NLB with TCP_QUIC protocol and targets with server IDs", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			By("checking service status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})

			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})

			By("verifying load balancer has TCP_QUIC protocol", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP_QUIC",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
					},
				}

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80": "TCP_QUIC",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying targets have QUIC server IDs", func() {
				targetGroups, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(targetGroups)).To(Equal(1))

				tgARN := awssdk.ToString(targetGroups[0].TargetGroupArn)
				err = verifier.VerifyTargetsHaveQUICServerIDs(ctx, tf, tgARN, int(numReplicas))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("NLB without QUIC (control test)", func() {
		var svc *corev1.Service

		BeforeEach(func() {
			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":   "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-scheme": "internet-facing",
			}
			svc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
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
		})

		It("Should create NLB with TCP protocol and targets without server IDs", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			By("checking service status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})

			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})

			By("verifying load balancer has TCP protocol", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
					},
				}

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying targets do not have QUIC server IDs", func() {
				targetGroups, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(targetGroups)).To(Equal(1))

				tgARN := awssdk.ToString(targetGroups[0].TargetGroupArn)
				Eventually(func() bool {
					targets, err := tf.TGManager.GetCurrentTargets(ctx, tgARN)
					if err != nil {
						return false
					}
					if len(targets) != int(numReplicas) {
						return false
					}
					for _, target := range targets {
						if target.Target.QuicServerId != nil {
							return false
						}
					}
					return true
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
		})
	})
})
