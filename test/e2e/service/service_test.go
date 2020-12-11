package service_test

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/service"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

const (
	defaultTestImage = "kishorj/hello-multi:v1"
	appContainerPort = 80
)

var _ = Describe("Service", func() {
	var (
		ctx         context.Context
		ns          *corev1.Namespace
		deployment  *appsv1.Deployment
		name        string
		numReplicas int32
		labels      map[string]string
	)

	BeforeSuite(func() {
		ctx = context.Background()
		name = utils.RandomDNS1123Label(20)
		numReplicas = 3
		var err error
		ns, err = tf.NSManager.AllocateNamespace(ctx, "service-e2e")
		Expect(err).ToNot(HaveOccurred())
		labels = map[string]string{
			"app.kubernetes.io/name":     "multi-port",
			"app.kubernetes.io/instance": name,
		}
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns.Name,
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
		tf.K8sClient.Create(ctx, deployment)
		tf.DPManager.WaitUntilDeploymentReady(ctx, deployment)
	})

	AfterSuite(func() {
		tf.K8sClient.Delete(ctx, deployment)
		tf.DPManager.WaitUntilDeploymentDeleted(ctx, deployment)
		tf.K8sClient.Delete(ctx, ns)
		tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
	})

	Context("NLB IP Load Balancer", func() {
		var (
			svcTest service.ServiceTest
			svc     *corev1.Service
		)
		BeforeEach(func() {
			svcTest = service.ServiceTest{}
			svc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ns.Name,
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
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
			By("Creating service", func() {
				err := svcTest.Create(ctx, tf, svc)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Verify Service with AWS", func() {
				err := svcTest.VerifyAWSLoadBalancerResources(ctx, tf, service.LoadBalancerExpectation{
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
					TargetGroupHC: &service.TargetGroupHC{
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
			By("Send traffic to LB", func() {
				err := svcTest.SendTrafficToLB(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Specifying Healthcheck annotations", func() {
				oldSvc := svc.DeepCopy()
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol"] = "HTTP"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-port"] = "80"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-path"] = "/healthz"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval"] = "30"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout"] = "6"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold"] = "2"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold"] = "2"

				err := svcTest.Update(ctx, tf, svc, oldSvc)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					return svcTest.GetTargetGroupHealthCheckProtocol(ctx, tf) == "HTTP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

				err = svcTest.VerifyAWSLoadBalancerResources(ctx, tf, service.LoadBalancerExpectation{
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
					TargetGroupHC: &service.TargetGroupHC{
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
			By("Deleting service", func() {
				err := svcTest.Cleanup(ctx, tf, svc)
				Expect(err).ToNot(HaveOccurred())
				newSvc := &corev1.Service{}
				err = tf.K8sClient.Get(ctx, k8s.NamespacedName(svc), newSvc)
				Expect(apierrs.IsNotFound(err)).To(BeTrue())
			})
		})
	})

	Context("NLB IP with TLS annotations", func() {
		var (
			svcTest service.ServiceTest
			svc     *corev1.Service
			certArn string
		)
		BeforeEach(func() {
			svc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name + "-tls",
					Namespace: ns.Name,
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
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
			certArn = svcTest.GenerateAndImportCertToACM(ctx, tf, "*.elb.us-west-2.amazonaws.com")
			Expect(certArn).ToNot(BeNil())
		})

		AfterEach(func() {
			Eventually(func() bool {
				return svcTest.DeleteCertFromACM(ctx, tf, certArn) != nil
			}, utils.PollTimeoutMedium, utils.PollIntervalLong).Should(BeTrue())
		})

		It("Should create TLS listeners", func() {
			By("Creating service", func() {
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-cert"] = certArn
				err := svcTest.Create(ctx, tf, svc)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Verifying AWS configuration", func() {
				err := svcTest.VerifyAWSLoadBalancerResources(ctx, tf, service.LoadBalancerExpectation{
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
				err := svcTest.SendTrafficToLB(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Specifying specific ports for SSL", func() {
				oldSvc := svc.DeepCopy()
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-ports"] = "443, 333"
				err := svcTest.Update(ctx, tf, svc, oldSvc)
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() bool {
					return svcTest.GetListenerProtocol(ctx, tf, "80") == "TCP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

				err = svcTest.VerifyAWSLoadBalancerResources(ctx, tf, service.LoadBalancerExpectation{
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
				oldSvc := svc.DeepCopy()
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-ports"] = "443, 333"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-ports"] = "443, http, 333"
				err := svcTest.Update(ctx, tf, svc, oldSvc)
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() bool {
					return svcTest.GetListenerProtocol(ctx, tf, "80") == "TLS"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("Specifying logging annotations", func() {
				oldSvc := svc.DeepCopy()
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-ports"] = "443, 333"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-access-log-enabled"] = "true"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name"] = "nlb-ip-svc-tls313"
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix"] = "nlb-pfx"
				err := svcTest.Update(ctx, tf, svc, oldSvc)
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() bool {
					return svcTest.VerifyLoadBalancerAttributes(ctx, tf, map[string]string{
						"access_logs.s3.enabled": "true",
						"access_logs.s3.bucket":  "nlb-ip-svc-tls313",
						"access_logs.s3.prefix":  "nlb-pfx",
					}) == nil
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
				// TODO: send traffic to the LB and verify access logs in S3
			})
			By("Deleting service", func() {
				err := svcTest.Cleanup(ctx, tf, svc)
				Expect(err).ToNot(HaveOccurred())
				newSvc := &corev1.Service{}
				err = tf.K8sClient.Get(ctx, k8s.NamespacedName(svc), newSvc)
				Expect(apierrs.IsNotFound(err)).To(BeTrue())
			})
		})
	})
})
