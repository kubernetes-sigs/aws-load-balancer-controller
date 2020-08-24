package service_test

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/test/e2e/service"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/test/framework"
	"time"
)

var _ = Describe("Service", func() {
	var (
		ctx context.Context
		f   *framework.Framework
	)

	BeforeSuite(func() {
		f = framework.New(framework.GlobalOptions)
		ctx = context.Background()
		Expect(f).ToNot(BeNil())
	})

	AfterSuite(func() {
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
					Name:      "nlb-ip-svc",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					// TODO: Create deployment from test
					Selector: map[string]string{"app": "hello"},
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
				err := svcTest.Create(ctx, f, svc)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Verify Service with AWS", func() {
				err := svcTest.CheckWithAWS(ctx, f, service.LoadBalancerExpectation{
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "ip",
					Listeners: map[string]string{
						"80": "TCP",
					},
					TargetGroups: map[string]string{
						"80": "TCP",
					},
					NumTargets: 3,
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("Send traffic to LB", func() {
				err := svcTest.SendTrafficToLB(ctx, f)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Deleting service", func() {
				err := svcTest.Cleanup(ctx, f)
				Expect(err).ToNot(HaveOccurred())
				newSvc := &corev1.Service{}
				err = f.K8sClient.Get(ctx, k8s.NamespacedName(svc), newSvc)
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
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "hello"},
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			}
			certArn = svcTest.GenerateAndImportCertToACM(ctx, f,"*.elb.us-west-2.amazonaws.com")
			Expect(certArn).ToNot(BeNil())
		})

		AfterEach(func() {
			err := svcTest.DeleteCertFromACM(ctx, f, certArn)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should create TLS listeners", func() {
			By("Creating service", func() {
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-cert"] = certArn
				err := svcTest.Create(ctx, f, svc)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Verify Service with AWS", func() {
				err := svcTest.CheckWithAWS(ctx, f, service.LoadBalancerExpectation{
					Type:       "network",
					Scheme:     "internet-facing",
					TargetType: "ip",
					Listeners: map[string]string{
						"80": "TLS",
					},
					TargetGroups: map[string]string{
						"80": "TCP",
					},
					NumTargets: 3,
				})
				Expect(err).ToNot(HaveOccurred())
			})
			By("Send traffic to LB", func() {
				err := svcTest.SendTrafficToLB(ctx, f)
				Expect(err).ToNot(HaveOccurred())
			})
			By("Specifying specific ports for SSL", func() {
				svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-ports"] = "443, 333"
				err := svcTest.Update(ctx, f, svc)
				Expect(err).ToNot(HaveOccurred())
				for i := 0; i < 10; i++ {
					err = svcTest.CheckWithAWS(ctx, f, service.LoadBalancerExpectation{
						Type:       "network",
						Scheme:     "internet-facing",
						TargetType: "ip",
						Listeners: map[string]string{
							"80": "TCP",
						},
						TargetGroups: map[string]string{
							"80": "TCP",
						},
						NumTargets: 3,
					})
					if err == nil {
						break
					}
					time.Sleep(10 * time.Second)
				}
				Expect(err).ToNot(HaveOccurred())
			})
			By("Deleting service", func() {
				err := svcTest.Cleanup(ctx, f)
				Expect(err).ToNot(HaveOccurred())
				newSvc := &corev1.Service{}
				err = f.K8sClient.Get(ctx, k8s.NamespacedName(svc), newSvc)
				Expect(apierrs.IsNotFound(err)).To(BeTrue())
			})
		})
	})
})
