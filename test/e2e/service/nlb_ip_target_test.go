package service

import (
	"context"
	"fmt"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("k8s service using ip target reconciled by the aws load balancer", func() {
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
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "app",
								ImagePullPolicy: corev1.PullAlways,
								Image:           dpImage,
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
			if tf.Options.IPFamily == framework.IPv6 {
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
				err := stack.Deploy(ctx, tf, svc, deployment, nil, nil, map[string]string{})
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

				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "TCP",
							Port:               "traffic-port",
							Interval:           10,
							Timeout:            10,
							HealthyThreshold:   3,
							UnhealthyThreshold: 3,
						},
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
				Expect(err).ToNot(HaveOccurred())
			})
			By("waiting for target group targets to be healthy", func() {
				err := verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, int(numReplicas))
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
					return verifier.GetTargetGroupHealthCheckProtocol(ctx, tf, lbARN) == "HTTP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "HTTP",
							Port:               "80",
							Path:               "/healthz",
							Interval:           30,
							Timeout:            6,
							HealthyThreshold:   2,
							UnhealthyThreshold: 2,
						},
					},
				}

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("Enabling Multi-cluster mode", func() {
				// Enable multicluster mode
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-multi-cluster-target-group": "true",
				})
				Expect(err).ToNot(HaveOccurred())

				// Wait for the change to be applied.
				time.Sleep(60 * time.Second)

				// Register a new target that exists outside the cluster.
				targetGroups, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)

				Expect(err).ToNot(HaveOccurred())

				tgArn := *targetGroups[0].TargetGroupArn

				Expect(targetGroups).To(HaveLen(1))
				targets, err := tf.TGManager.GetCurrentTargets(ctx, tgArn)
				Expect(err).ToNot(HaveOccurred())
				Expect(targets).ShouldNot(HaveLen(0))

				err = tf.TGManager.RegisterTargets(ctx, tgArn, []elbv2types.TargetDescription{
					{
						Id:   targets[0].Target.Id,
						Port: awssdk.Int32(*targets[0].Target.Port + 1),
					},
				})

				Expect(err).ToNot(HaveOccurred())

				// Change the check point annotation to trigger a reconcile.
				err = stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"elbv2.k8s.aws/checkpoint": "baz",
				})

				Expect(err).ToNot(HaveOccurred())

				// Wait for the change to be applied.
				time.Sleep(120 * time.Second)

				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas) + 1,
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "HTTP",
							Port:               "80",
							Path:               "/healthz",
							Interval:           30,
							Timeout:            6,
							HealthyThreshold:   2,
							UnhealthyThreshold: 2,
						},
					},
				}

				// We should the targets registered from in cluster and the extra IP registered under a different port.
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).ToNot(HaveOccurred())

				// Disable multicluster mode
				err = stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-multi-cluster-target-group": "false",
				})
				Expect(err).ToNot(HaveOccurred())

				// Wait for the change to be applied.
				time.Sleep(120 * time.Second)

				// Only the replicas in the cluster should exist in the target group again.

				expectedTargetGroups = []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "HTTP",
							Port:               "80",
							Path:               "/healthz",
							Interval:           30,
							Timeout:            6,
							HealthyThreshold:   2,
							UnhealthyThreshold: 2,
						},
					},
				}

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: expectedTargetGroups,
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
			if tf.Options.IPFamily == framework.IPv6 {
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
						{
							Port:       443,
							Name:       "https",
							TargetPort: intstr.FromInt(443),
							Protocol:   corev1.ProtocolTCP,
						},
						{
							Port:       333,
							Name:       "arbitrary-port",
							TargetPort: intstr.FromInt(333),
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
				err := stack.Deploy(ctx, tf, svc, deployment, nil, nil, map[string]string{})
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

			expectedTargetGroups := []verifier.ExpectedTargetGroup{
				{
					Protocol:   "TCP",
					Port:       80,
					NumTargets: int(numReplicas),
					TargetType: "ip",
				},
				{
					Protocol:   "TCP",
					Port:       443,
					NumTargets: int(numReplicas),
					TargetType: "ip",
				},
				{
					Protocol:   "TCP",
					Port:       333,
					NumTargets: int(numReplicas),
					TargetType: "ip",
				},
			}

			By("Verifying AWS configuration", func() {
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80":  "TLS",
						"443": "TLS",
						"333": "TLS",
					},
					TargetGroups: expectedTargetGroups,
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
					return verifier.GetLoadBalancerListenerProtocol(ctx, tf, lbARN, "80") == "TCP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

				expectedTargetGroups = []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
					},
					{
						Protocol:   "TCP",
						Port:       443,
						NumTargets: int(numReplicas),
						TargetType: "ip",
					},
					{
						Protocol:   "TCP",
						Port:       333,
						NumTargets: int(numReplicas),
						TargetType: "ip",
					},
				}

				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80":  "TCP",
						"443": "TLS",
						"333": "TLS",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("Including service port in ssl-ports annotation", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-ssl-ports": "443, http, 333",
				})
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() bool {
					return verifier.GetLoadBalancerListenerProtocol(ctx, tf, lbARN, "80") == "TLS"
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
					return verifier.VerifyLoadBalancerAttributes(ctx, tf, lbARN, map[string]string{
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
			if tf.Options.IPFamily == framework.IPv6 {
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
				err := stack.Deploy(ctx, tf, svc, deployment, nil, nil, map[string]string{})
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
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "TCP",
							Port:               "traffic-port",
							Interval:           10,
							Timeout:            10,
							HealthyThreshold:   3,
							UnhealthyThreshold: 3,
						},
					},
				}

				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Name:   lbName,
					Type:   "network",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: expectedTargetGroups,
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("waiting for load balancer to be available", func() {
				err := tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, lbARN)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("NLB IP with weighted target groups", func() {
		var (
			lbTypeSvcs     []*corev1.Service
			nonLbTypeSvcs  []*corev1.Service
			svc            *corev1.Service
			svcName        string
			anotherSvc     *corev1.Service
			anotherSvcName string
			baseSvcWeight  int32
			svc1Name       string
			svc1Weight     int32
			targetSvc1     *corev1.Service
			svc2Name       string
			svc2Weight     int32
			targetSvc2     *corev1.Service
			dnsName1       string
			dnsName2       string
			lbARN1         string
			lbARN2         string
		)
		BeforeEach(func() {
			lbTypeSvcs = []*corev1.Service{}
			nonLbTypeSvcs = []*corev1.Service{}
			if strings.HasPrefix(tf.Options.AWSRegion, "cn-") || strings.Contains(tf.Options.AWSRegion, "-iso-") || tf.Options.AWSRegion == "eusc-de-east-1" {
				Skip("Skipping test, weighted target groups not supported in this region")
			}
			// Service 1 to forward to
			svc1Name = fmt.Sprintf("target-svc1-%v", utils.RandomDNS1123Label(5))
			targetSvc1 = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: svc1Name,
				},
				Spec: corev1.ServiceSpec{
					Selector: labels,
					Type:     corev1.ServiceTypeNodePort,
					Ports: []corev1.ServicePort{
						{
							Port:     81,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}

			// Service 2 to forward to
			svc2Name = fmt.Sprintf("target-svc2-%v", utils.RandomDNS1123Label(5))
			targetSvc2 = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: svc2Name,
				},
				Spec: corev1.ServiceSpec{
					Selector: labels,
					Type:     corev1.ServiceTypeNodePort,
					Ports: []corev1.ServicePort{
						{
							Port:     82,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}

			baseSvcWeight = 20
			svc1Weight = 60
			svc2Weight = 40
			forwardActionValue := fmt.Sprintf(
				`{
								"type": "forward",
								"forwardConfig": {
									"baseServiceWeight": %d,
									"targetGroups": [
										{
											"serviceName": "%v",
											"servicePort": 81,
											"weight": %d,
											"tcpEnabled": true
										},
										{
											"serviceName": "%v",
											"servicePort": 82,
											"weight": %d
										}
									]
								}
							}`, baseSvcWeight, svc1Name, svc1Weight, svc2Name, svc2Weight)

			annotation := map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":   "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-scheme": "internet-facing",
				"service.beta.kubernetes.io/actions.TCP-80":           forwardActionValue,
			}

			if tf.Options.IPFamily == framework.IPv6 {
				annotation["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
			}

			svcName = "my-nlb"
			svc = &corev1.Service{
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
			anotherSvcName = "my-another-nlb"
			anotherSvc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        anotherSvcName,
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
			lbTypeSvcs = append(lbTypeSvcs, anotherSvc)
			nonLbTypeSvcs = append(nonLbTypeSvcs, targetSvc1, targetSvc2)
		})
		It("Should create and verify service", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment, nil, nonLbTypeSvcs, map[string]string{})
				Expect(err).NotTo(HaveOccurred())
			})
			By("checking service status for lb dns name", func() {
				dnsName = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName).ToNot(BeEmpty())
			})
			By("querying AWS load balancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).ToNot(BeEmpty())
			})
			By("verifying service with AWS", func() {
				expectedTargetGroups := []verifier.ExpectedTargetGroup{
					{
						// Base service target group
						Protocol:   "TCP",
						Port:       80,
						NumTargets: int(numReplicas),
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "TCP",
							Port:               "traffic-port",
							Interval:           10,
							Timeout:            10,
							HealthyThreshold:   3,
							UnhealthyThreshold: 3,
						},
					},
					{
						// Target service 1
						Protocol:   "TCP",
						Port:       81,
						NumTargets: int(numReplicas),
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "TCP",
							Port:               "traffic-port",
							Interval:           10,
							Timeout:            10,
							HealthyThreshold:   3,
							UnhealthyThreshold: 3,
						},
					},
					{
						// Target service 2
						Protocol:   "TCP",
						Port:       82,
						NumTargets: int(numReplicas),
						TargetType: "ip",
						TargetGroupHC: &verifier.TargetGroupHC{
							Protocol:           "TCP",
							Port:               "traffic-port",
							Interval:           10,
							Timeout:            10,
							HealthyThreshold:   3,
							UnhealthyThreshold: 3,
						},
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
				Expect(err).ToNot(HaveOccurred())
			})
		})

		It("Should support multiple NLB services sharing the same backend services", func() {
			By("deploying external NLB stack", func() {
				err := stack.Deploy(ctx, tf, svc, deployment, lbTypeSvcs, nonLbTypeSvcs, map[string]string{})
				Expect(err).ToNot(HaveOccurred())
				dnsName1 = stack.GetLoadBalancerIngressHostName()
				Expect(dnsName1).ToNot(BeEmpty())
				observedAnotherService, err := stack.resourceStack.waitUntilServiceReady(ctx, tf, anotherSvc)
				Expect(err).NotTo(HaveOccurred())
				dnsName2 = stack.resourceStack.GetLoadBalancerIngressHostnameForService(observedAnotherService)
				Expect(dnsName2).ToNot(BeEmpty())
				lbARN1, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName1)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN1).ToNot(BeEmpty())
				lbARN2, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName2)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN2).ToNot(BeEmpty())
				// Get target groups for first load balancer
				targetGroups1, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN1)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(targetGroups1)).To(Equal(3)) // Base service + 2 backend services

				// Get target groups for second load balancer
				targetGroups2, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN2)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(targetGroups2)).To(Equal(3)) // Base service + 2 backend services

				// Extract the ARNs for comparison
				tgARNs1 := make([]string, len(targetGroups1))
				for i, tg := range targetGroups1 {
					tgARNs1[i] = awssdk.ToString(tg.TargetGroupArn)
				}

				tgARNs2 := make([]string, len(targetGroups2))
				for i, tg := range targetGroups2 {
					tgARNs2[i] = awssdk.ToString(tg.TargetGroupArn)
				}

				// Verify the two load balancers have different target groups
				// (each has its own set of target groups, even though they reference the same backends)
				for _, arn1 := range tgARNs1 {
					for _, arn2 := range tgARNs2 {
						Expect(arn1).ToNot(Equal(arn2))
					}
				}
			})
		})
	})
})
