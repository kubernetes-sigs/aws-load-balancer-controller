package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	apierrs "k8s.io/apimachinery/pkg/api/errors"

	networking "k8s.io/api/networking/v1"

	"github.com/gavv/httpexpect/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/fixture"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/manifest"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("certificate management ingress tests", func() {
	var (
		ctx context.Context
		// sandbox namespace
		sandboxNS *corev1.Namespace
	)

	exact := networking.PathTypeExact

	BeforeEach(func() {
		ctx = context.Background()

		if !tf.Options.EnableCertMgmtTests {
			Skip("Skipping certificiate management tests (not enabled)")
		}

		By("setup sandbox namespace", func() {
			tf.Logger.Info("allocating namespace")
			ns, err := tf.NSManager.AllocateNamespace(ctx, "aws-lb-e2e-cert-mgmt")
			Expect(err).NotTo(HaveOccurred())
			tf.Logger.Info("allocated namespace", "name", ns.Name)
			sandboxNS = ns
		})
	})

	AfterEach(func() {
		if sandboxNS != nil {
			By("teardown sandbox namespace", func() {
				{
					tf.Logger.Info("deleting namespace", "name", sandboxNS.Name)
					err := tf.K8sClient.Delete(ctx, sandboxNS)
					Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrs.IsNotFound)))
					tf.Logger.Info("deleted namespace", "name", sandboxNS.Name)
				}
				{
					tf.Logger.Info("waiting namespace becomes deleted", "name", sandboxNS.Name)
					err := tf.NSManager.WaitUntilNamespaceDeleted(ctx, sandboxNS)
					Expect(err).NotTo(HaveOccurred())
					tf.Logger.Info("namespace becomes deleted", "name", sandboxNS.Name)
				}
			})
		}
	})

	Context("using amazon issued certificates", func() {
		BeforeEach(func() {
			if tf.Options.Route53ValidationDomain == "" {
				Skip("Skipping cert mgmt tests with amazon issued CA (no Route53 domain provided)")
			}
		})

		It("basic ingress", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app", tf.Options.TestImageRegistry)
			ingBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			ingClass := &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: sandboxNS.Name,
				},
				Spec: networking.IngressClassSpec{
					Controller: "ingress.k8s.aws/alb",
				},
			}
			annotation := map[string]string{
				"alb.ingress.kubernetes.io/scheme":          "internet-facing",
				"alb.ingress.kubernetes.io/target-type":     "ip",
				"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
				"alb.ingress.kubernetes.io/create-acm-cert": "true",
			}
			ing := ingBuilder.
				AddHTTPRoute(tf.Options.Route53ValidationDomain, networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithIngressClassName(ingClass.Name).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ingClass, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			certARN, _ := ExpectOneCertProvisionedForIngress(ctx, tf, ing)
			ExpectCertTypeToBe(ctx, certARN, types.CertificateTypeAmazonIssued)
			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)
			ExpectCertToBeInStatus(ctx, certARN, types.CertificateStatusIssued)
			ExpectCertToBeInUse(ctx, certARN)

			// test traffic (http)
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpsExp := httpexpect.WithConfig(httpexpect.Config{
				Reporter: tf.LoggerReporter,
				Client: &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							// accept any certificate; for testing only! (the dns name of the LB doesn't match the domain we issued a cert for)
							InsecureSkipVerify: true,
						},
					},
				},
				BaseURL: fmt.Sprintf("https://%s", lbDNS),
			})
			httpsExp.GET("/path").WithHeader("Host", tf.Options.Route53ValidationDomain).Expect().
				Status(http.StatusOK).Body().Equal("Hello World!")
		})
	})

	Context("using PCA issued certificate", func() {
		BeforeEach(func() {
			if tf.Options.PCAARN == "" {
				Skip("Skipping cert mgmt tests with private CA (no PCA ARN provided)")
			}
		})

		It("basic ingress", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app", tf.Options.TestImageRegistry)
			ingBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			ingClass := &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: sandboxNS.Name,
				},
				Spec: networking.IngressClassSpec{
					Controller: "ingress.k8s.aws/alb",
				},
			}
			annotation := map[string]string{
				"alb.ingress.kubernetes.io/scheme":          "internet-facing",
				"alb.ingress.kubernetes.io/target-type":     "ip",
				"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
				"alb.ingress.kubernetes.io/create-acm-cert": "true",
				"alb.ingress.kubernetes.io/acm-pca-arn":     tf.Options.PCAARN,
			}
			ing := ingBuilder.
				AddHTTPRoute("example.com", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithIngressClassName(ingClass.Name).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ingClass, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			certARN, _ := ExpectOneCertProvisionedForIngress(ctx, tf, ing)
			ExpectCertTypeToBe(ctx, certARN, types.CertificateTypePrivate)
			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)
			ExpectCertToBeInStatus(ctx, certARN, types.CertificateStatusIssued)
			ExpectCertToBeInUse(ctx, certARN)

			// test traffic (http)
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpsExp := httpexpect.WithConfig(httpexpect.Config{
				Reporter: tf.LoggerReporter,
				Client: &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							// accept any certificate; for testing only! (the dns name of the LB doesn't match the domain we issued a cert for)
							InsecureSkipVerify: true,
						},
					},
				},
				BaseURL: fmt.Sprintf("https://%s", lbDNS),
			})
			httpsExp.GET("/path").WithHeader("Host", "example.com").Expect().
				Status(http.StatusOK).Body().Equal("Hello World!")
		})
	})
})

// ExpectOneCertProvisionedForIngress expects one Certificate provisioned for Ingress
func ExpectOneCertProvisionedForIngress(ctx context.Context, tf *framework.Framework, ing *networking.Ingress) (certARN string, hosts []string) {
	Eventually(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, k8s.NamespacedName(ing), ing)
		g.Expect(err).NotTo(HaveOccurred())
		hosts = FindIngressHostnames(ing)
		g.Expect(hosts).ShouldNot(BeEmpty())
		certARN, err = tf.CertManager.FindCertificateByHostnames(ctx, hosts)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(certARN).ShouldNot(BeEmpty())
	}, utils.CertReconcileTimeout, utils.PollIntervalMedium).Should(Succeed())

	tf.Logger.Info("Cert provisioned", "arn", certARN)

	return certARN, hosts
}

func ExpectCertToBeInUse(ctx context.Context, certARN string) {
	detail, err := tf.CertManager.GetCertificateDetail(ctx, certARN)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(detail.InUseBy)).ToNot(Equal(0))

	tf.Logger.Info("Cert in use", "arn", certARN, "inUseBy", detail.InUseBy)
}

func ExpectCertToBeInStatus(ctx context.Context, certARN string, status types.CertificateStatus) {
	Eventually(func(g Gomega) {
		detail, err := tf.CertManager.GetCertificateDetail(ctx, certARN)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(detail.Status).To(Equal(status))
	}, utils.CertReconcileTimeout, utils.PollIntervalLong).Should(Succeed())

	tf.Logger.Info("Cert in expected status", "arn", certARN, "status", string(status))
}

func ExpectCertTypeToBe(ctx context.Context, certARN string, t types.CertificateType) {
	detail, err := tf.CertManager.GetCertificateDetail(ctx, certARN)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(detail.Type)).ToNot(Equal(t))

	tf.Logger.Info("Cert of expected type", "arn", certARN, "type", detail.Type)
}
