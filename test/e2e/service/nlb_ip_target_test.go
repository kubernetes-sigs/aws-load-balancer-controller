package service

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("k8s service reconciled by the aws load balancer", func() {
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
		numReplicas = 3
		stack = NLBIPTestStack{}
		name = "ip-e2e"
		labels = map[string]string{
			"app.kubernetes.io/name":     "multi-port",
			"app.kubernetes.io/instance": name,
		}
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
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "app",
								ImagePullPolicy: corev1.PullAlways,
								Image:           defaultTestImage,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: appContainerPort,
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

	Context("NLB with IP target configuration", func() {
		var (
			svc *corev1.Service
		)
		BeforeEach(func() {
			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":   "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-scheme": "internet-facing",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
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
		It("Should create and verify internet-facing NLB with IP targets", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment)
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
			By("Verify Service with AWS", func() {
				err := verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "ip",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: map[string]string{
						"80": "TCP",
					},
					NumTargets: int(numReplicas),
					TargetGroupHC: &TargetGroupHC{
						Protocol:           "TCP",
						Port:               "traffic-port",
						Interval:           10,
						Timeout:            10,
						HealthyThreshold:   3,
						UnhealthyThreshold: 3,
					},
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("waiting for target group targets to be healthy", func() {
				err := waitUntilTargetsAreHealthy(ctx, tf, lbARN, int(numReplicas))
				Expect(err).NotTo(HaveOccurred())
			})
			By("Send traffic to LB", func() {
				err := stack.SendTrafficToLB(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Specifying Healthcheck annotations", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "HTTP",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "80",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                "/healthz",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "30",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "6",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "2",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "2",
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					return getTargetGroupHealthCheckProtocol(ctx, tf, lbARN) == "HTTP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

				err = verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "ip",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: map[string]string{
						"80": "TCP",
					},
					NumTargets: int(numReplicas),
					TargetGroupHC: &TargetGroupHC{
						Protocol:           "HTTP",
						Port:               "80",
						Path:               "/healthz",
						Interval:           30,
						Timeout:            6,
						HealthyThreshold:   2,
						UnhealthyThreshold: 2,
					},
				})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("NLB IP with TLS configuration", func() {
		var (
			svc *corev1.Service
		)
		BeforeEach(func() {
			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":     "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-scheme":   "internet-facing",
				"service.beta.kubernetes.io/aws-load-balancer-ssl-cert": tf.Options.CertificateARNs,
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
			}
			svc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name + "-tls",
					Annotations: annotation,
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: labels,
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							Name:       "http",
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			}
		})
		It("Should create TLS listeners", func() {
			if len(tf.Options.CertificateARNs) == 0 {
				Skip("Skipping tests, certificates not specified")
			}
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment)
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
			By("Verifying AWS configuration", func() {
				err := verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "ip",
					Listeners: map[string]string{
						"80": "TLS",
					},
					TargetGroups: map[string]string{
						"80": "TCP",
					},
					NumTargets: int(numReplicas),
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("Sending traffic to LB", func() {
				err := stack.SendTrafficToLB(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Specifying specific ports for SSL", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-ssl-ports": "443, 333",
				})
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() bool {
					return getLoadBalancerListenerProtocol(ctx, tf, lbARN, "80") == "TCP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

				err = verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "ip",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: map[string]string{
						"80": "TCP",
					},
					NumTargets: int(numReplicas),
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("Including service port in ssl-ports annotation", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-ssl-ports": "443, http, 333",
				})
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() bool {
					return getLoadBalancerListenerProtocol(ctx, tf, lbARN, "80") == "TLS"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("Specifying logging annotations", func() {
				if len(tf.Options.S3BucketName) == 0 {
					return
				}
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-ssl-ports":                   "443, 333",
					"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":          "true",
					"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":   tf.Options.S3BucketName,
					"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix": "nlb-pfx",
				})
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() bool {
					return verifyLoadBalancerAttributes(ctx, tf, lbARN, map[string]string{
						"access_logs.s3.enabled": "true",
						"access_logs.s3.bucket":  tf.Options.S3BucketName,
						"access_logs.s3.prefix":  "nlb-pfx",
					}) == nil
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
				// TODO: send traffic to the LB and verify access logs in S3
			})
		})
	})
	Context("NLB IP Load Balancer with name", func() {
		var (
			svc    *corev1.Service
			lbName string
		)
		lbName = utils.RandomDNS1123Label(20)
		BeforeEach(func() {
			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-name":   lbName,
				"service.beta.kubernetes.io/aws-load-balancer-type":   "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-scheme": "internet-facing",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
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
		It("Should create and verify service", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment)
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
			By("Verify Service with AWS", func() {
				err := verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Name:       lbName,
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "ip",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: map[string]string{
						"80": "TCP",
					},
					NumTargets: int(numReplicas),
					TargetGroupHC: &TargetGroupHC{
						Protocol:           "TCP",
						Port:               "traffic-port",
						Interval:           10,
						Timeout:            10,
						HealthyThreshold:   3,
						UnhealthyThreshold: 3,
					},
				})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
