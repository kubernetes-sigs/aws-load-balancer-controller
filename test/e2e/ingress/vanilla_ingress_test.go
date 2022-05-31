package ingress

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/fixture"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/manifest"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("vanilla ingress tests", func() {
	var (
		ctx context.Context
		// sandbox namespace
		sandboxNS *corev1.Namespace
	)

	exact := networking.PathTypeExact

	BeforeEach(func() {
		ctx = context.Background()
		if tf.Options.ControllerImage != "" {
			By(fmt.Sprintf("ensure cluster installed with controller: %s", tf.Options.ControllerImage), func() {
				err := tf.CTRLInstallationManager.UpgradeController(tf.Options.ControllerImage)
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(60 * time.Second)
			})
		}

		By("setup sandbox namespace", func() {
			tf.Logger.Info("allocating namespace")
			ns, err := tf.NSManager.AllocateNamespace(ctx, "aws-lb-e2e")
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

	Context("with basic settings", func() {
		It("[ingress-class] with IngressClass configured with 'ingress.k8s.aws/alb' controller, one ALB shall be created and functional", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app")
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
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
				annotation["alb.ingress.kubernetes.io/target-type"] = "ip"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithIngressClassName(ingClass.Name).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ingClass, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)
			// test traffic
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpExp := httpexpect.New(tf.Logger, fmt.Sprintf("http://%v", lbDNS))
			httpExp.GET("/path").Expect().
				Status(http.StatusOK).
				Body().Equal("Hello World!")
		})

		It("with 'kubernetes.io/ingress.class' annotation set to 'alb', one ALB shall be created and functional", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app")
			ingBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			annotation := map[string]string{
				"kubernetes.io/ingress.class":      "alb",
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
				annotation["alb.ingress.kubernetes.io/target-type"] = "ip"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)
			// test traffic
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpExp := httpexpect.New(tf.Logger, fmt.Sprintf("http://%v", lbDNS))
			httpExp.GET("/path").Expect().
				Status(http.StatusOK).
				Body().Equal("Hello World!")
		})
	})

	Context("with IngressClass variant settings", func() {
		It("[ingress-class] with IngressClass configured with 'nginx' controller, no ALB shall be created", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app")
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
					Controller: "kubernetes.io/nginx",
				},
			}
			annotation := map[string]string{
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithIngressClassName(ingClass.Name).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ingClass, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			ExpectNoLBProvisionedForIngress(ctx, tf, ing)
		})

		It("with 'kubernetes.io/ingress.class' annotation set to 'nginx', no ALB shall be created", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app")
			ingBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			annotation := map[string]string{
				"kubernetes.io/ingress.class":      "nginx",
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			ExpectNoLBProvisionedForIngress(ctx, tf, ing)
		})

		It("without IngressClass or 'kubernetes.io/ingress.class' annotation, no ALB shall be created", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app")
			ingBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			annotation := map[string]string{
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			ExpectNoLBProvisionedForIngress(ctx, tf, ing)
		})
	})

	Context("with `alb.ingress.kubernetes.io/load-balancer-name` variant settings", func() {
		It("with 'alb.ingress.kubernetes.io/load-balancer-name' annotation explicitly specified, one ALB shall be created and functional", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app")
			ingBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			safeClusterName := strings.ReplaceAll(tf.Options.ClusterName, ".", "-")
			lbName := fmt.Sprintf("%.16s-%.15s", safeClusterName, sandboxNS.Name)
			annotation := map[string]string{
				"kubernetes.io/ingress.class":                  "alb",
				"alb.ingress.kubernetes.io/scheme":             "internet-facing",
				"alb.ingress.kubernetes.io/load-balancer-name": lbName,
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
				annotation["alb.ingress.kubernetes.io/target-type"] = "ip"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)

			sdkLB, err := tf.LBManager.GetLoadBalancerFromARN(ctx, lbARN)
			Expect(err).NotTo(HaveOccurred())
			Expect(awssdk.StringValue(sdkLB.LoadBalancerName)).Should(Equal(lbName))

			// test traffic
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpExp := httpexpect.New(tf.Logger, fmt.Sprintf("http://%v", lbDNS))
			httpExp.GET("/path").Expect().
				Status(http.StatusOK).
				Body().Equal("Hello World!")
		})
	})

	Context("with ALB IP targets and named target port", func() {
		It("with 'alb.ingress.kubernetes.io/target-type' annotation explicitly specified, one ALB shall be created and functional", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder().WithTargetPortName("e2e-targetport")
			ingBuilder := manifest.NewIngressBuilder()
			dp, svc := appBuilder.Build(sandboxNS.Name, "app")
			ingBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			annotation := map[string]string{
				"kubernetes.io/ingress.class":           "alb",
				"alb.ingress.kubernetes.io/scheme":      "internet-facing",
				"alb.ingress.kubernetes.io/target-type": "ip",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp, svc, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)

			// test traffic
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpExp := httpexpect.New(tf.Logger, fmt.Sprintf("http://%v", lbDNS))
			httpExp.GET("/path").Expect().
				Status(http.StatusOK).
				Body().Equal("Hello World!")
		})
	})

	Context("with `alb.ingress.kubernetes.io/actions.${action-name}` variant settings", func() {
		It("with annotation based actions, one ALB shall be created and functional", func() {
			appBuilder := manifest.NewFixedResponseServiceBuilder()
			ingBuilder := manifest.NewIngressBuilder()
			dp1, svc1 := appBuilder.WithHTTPBody("app-1").Build(sandboxNS.Name, "app-1")
			dp2, svc2 := appBuilder.WithHTTPBody("app-2").Build(sandboxNS.Name, "app-2")
			ingResponse503Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "response-503",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingRedirectToAWSBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "redirect-to-aws",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingForwardSingleTGBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "forward-single-tg",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingForwardMultipleTGBackend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "forward-multiple-tg",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			annotation := map[string]string{
				"kubernetes.io/ingress.class":                           "alb",
				"alb.ingress.kubernetes.io/scheme":                      "internet-facing",
				"alb.ingress.kubernetes.io/actions.response-503":        "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"503\",\"messageBody\":\"503 error text\"}}",
				"alb.ingress.kubernetes.io/actions.redirect-to-aws":     "{\"type\":\"redirect\",\"redirectConfig\":{\"host\":\"aws.amazon.com\",\"path\":\"/eks/\",\"port\":\"443\",\"protocol\":\"HTTPS\",\"query\":\"k=v\",\"statusCode\":\"HTTP_302\"}}",
				"alb.ingress.kubernetes.io/actions.forward-single-tg":   "{\"type\":\"forward\",\"forwardConfig\":{\"targetGroups\":[{\"serviceName\":\"app-1\",\"servicePort\":\"80\"}]}}",
				"alb.ingress.kubernetes.io/actions.forward-multiple-tg": "{\"type\":\"forward\",\"forwardConfig\":{\"targetGroups\":[{\"serviceName\":\"app-1\",\"servicePort\":\"80\",\"weight\":20},{\"serviceName\":\"app-2\",\"servicePort\":80,\"weight\":80}],\"targetGroupStickinessConfig\":{\"enabled\":true,\"durationSeconds\":200}}}",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
				annotation["alb.ingress.kubernetes.io/target-type"] = "ip"
			}
			ing := ingBuilder.
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/response-503", PathType: &exact, Backend: ingResponse503Backend}).
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/redirect-to-aws", PathType: &exact, Backend: ingRedirectToAWSBackend}).
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/forward-single-tg", PathType: &exact, Backend: ingForwardSingleTGBackend}).
				AddHTTPRoute("", networking.HTTPIngressPath{Path: "/forward-multiple-tg", PathType: &exact, Backend: ingForwardMultipleTGBackend}).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, dp1, svc1, dp2, svc2, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)
			// test traffic
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpExp := httpexpect.New(tf.Logger, fmt.Sprintf("http://%v", lbDNS))
			httpExp.GET("/response-503").Expect().
				Status(http.StatusServiceUnavailable).
				Body().Equal("503 error text")
			httpExp.GET("/redirect-to-aws").WithRedirectPolicy(httpexpect.DontFollowRedirects).Expect().
				Status(http.StatusFound).
				Header("Location").Equal("https://aws.amazon.com:443/eks/?k=v")
			httpExp.GET("/forward-single-tg").Expect().
				Status(http.StatusOK).
				Body().Equal("app-1")
			httpExp.GET("/forward-multiple-tg").Expect().
				Status(http.StatusOK).
				Body().Match("app-1|app-2")
		})
	})

	Context("with `alb.ingress.kubernetes.io/conditions.${conditions-name}` variant settings", func() {
		It("with annotation based conditions, one ALB shall be created and functional", func() {
			ingBuilder := manifest.NewIngressBuilder()
			ingRulePath1Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "rule-path1",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingRulePath2Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "rule-path2",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingRulePath3Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "rule-path3",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingRulePath4Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "rule-path4",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingRulePath5Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "rule-path5",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingRulePath6Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "rule-path6",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}
			ingRulePath7Backend := networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "rule-path7",
					Port: networking.ServiceBackendPort{
						Name: "use-annotation",
					},
				},
			}

			annotation := map[string]string{
				"kubernetes.io/ingress.class":                     "alb",
				"alb.ingress.kubernetes.io/scheme":                "internet-facing",
				"alb.ingress.kubernetes.io/actions.rule-path1":    "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"200\",\"messageBody\":\"Host is www.example.com OR anno.example.com\"}}",
				"alb.ingress.kubernetes.io/conditions.rule-path1": "[{\"field\":\"host-header\",\"hostHeaderConfig\":{\"values\":[\"anno.example.com\"]}}]",
				"alb.ingress.kubernetes.io/actions.rule-path2":    "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"200\",\"messageBody\":\"Path is /path2 OR /anno/path2\"}}",
				"alb.ingress.kubernetes.io/conditions.rule-path2": "[{\"field\":\"path-pattern\",\"pathPatternConfig\":{\"values\":[\"/anno/path2\"]}}]",
				"alb.ingress.kubernetes.io/actions.rule-path3":    "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"200\",\"messageBody\":\"Http header HeaderName is HeaderValue1 OR HeaderValue2\"}}",
				"alb.ingress.kubernetes.io/conditions.rule-path3": "[{\"field\":\"http-header\",\"httpHeaderConfig\":{\"httpHeaderName\": \"HeaderName\", \"values\":[\"HeaderValue1\", \"HeaderValue2\"]}}]",
				"alb.ingress.kubernetes.io/actions.rule-path4":    "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"200\",\"messageBody\":\"Http request method is GET OR HEAD\"}}",
				"alb.ingress.kubernetes.io/conditions.rule-path4": "[{\"field\":\"http-request-method\",\"httpRequestMethodConfig\":{\"Values\":[\"GET\", \"HEAD\"]}}]",
				"alb.ingress.kubernetes.io/actions.rule-path5":    "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"200\",\"messageBody\":\"Query string is paramA:valueA1 OR paramA:valueA2\"}}",
				"alb.ingress.kubernetes.io/conditions.rule-path5": "[{\"field\":\"query-string\",\"queryStringConfig\":{\"values\":[{\"key\":\"paramA\",\"value\":\"valueA1\"},{\"key\":\"paramA\",\"value\":\"valueA2\"}]}}]",
				"alb.ingress.kubernetes.io/actions.rule-path6":    "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"200\",\"messageBody\":\"Source IP is 192.168.0.0/16 OR 172.16.0.0/16\"}}",
				"alb.ingress.kubernetes.io/conditions.rule-path6": "[{\"field\":\"source-ip\",\"sourceIpConfig\":{\"values\":[\"192.168.0.0/16\", \"172.16.0.0/16\"]}}]",
				"alb.ingress.kubernetes.io/actions.rule-path7":    "{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"200\",\"messageBody\":\"multiple conditions applies\"}}",
				"alb.ingress.kubernetes.io/conditions.rule-path7": "[{\"field\":\"http-header\",\"httpHeaderConfig\":{\"httpHeaderName\": \"HeaderName\", \"values\":[\"HeaderValue\"]}},{\"field\":\"query-string\",\"queryStringConfig\":{\"values\":[{\"key\":\"paramA\",\"value\":\"valueA\"}]}},{\"field\":\"query-string\",\"queryStringConfig\":{\"values\":[{\"key\":\"paramB\",\"value\":\"valueB\"}]}}]",
			}
			if tf.Options.IPFamily == "IPv6" {
				annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
				annotation["alb.ingress.kubernetes.io/target-type"] = "ip"
			}
			ing := ingBuilder.
				AddHTTPRoute("www.example.com", networking.HTTPIngressPath{Path: "/path1", PathType: &exact, Backend: ingRulePath1Backend}).
				AddHTTPRoute("www.example.com", networking.HTTPIngressPath{Path: "/path2", PathType: &exact, Backend: ingRulePath2Backend}).
				AddHTTPRoute("www.example.com", networking.HTTPIngressPath{Path: "/path3", PathType: &exact, Backend: ingRulePath3Backend}).
				AddHTTPRoute("www.example.com", networking.HTTPIngressPath{Path: "/path4", PathType: &exact, Backend: ingRulePath4Backend}).
				AddHTTPRoute("www.example.com", networking.HTTPIngressPath{Path: "/path5", PathType: &exact, Backend: ingRulePath5Backend}).
				AddHTTPRoute("www.example.com", networking.HTTPIngressPath{Path: "/path6", PathType: &exact, Backend: ingRulePath6Backend}).
				AddHTTPRoute("www.example.com", networking.HTTPIngressPath{Path: "/path7", PathType: &exact, Backend: ingRulePath7Backend}).
				WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
			resStack := fixture.NewK8SResourceStack(tf, ing)
			err := resStack.Setup(ctx)
			Expect(err).NotTo(HaveOccurred())

			defer resStack.TearDown(ctx)

			lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)
			// test traffic
			ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
			httpExp := httpexpect.New(tf.Logger, fmt.Sprintf("http://%v", lbDNS))
			httpExp.GET("/path1").WithHost("www.example.com").Expect().
				Status(http.StatusOK).
				Body().Equal("Host is www.example.com OR anno.example.com")
			httpExp.GET("/path1").WithHost("anno.example.com").Expect().
				Status(http.StatusOK).
				Body().Equal("Host is www.example.com OR anno.example.com")
			httpExp.GET("/path1").WithHost("other.example.com").Expect().
				Status(http.StatusNotFound)

			httpExp.GET("/path2").WithHost("www.example.com").Expect().
				Status(http.StatusOK).
				Body().Equal("Path is /path2 OR /anno/path2")
			httpExp.GET("/anno/path2").WithHost("www.example.com").Expect().
				Status(http.StatusOK).
				Body().Equal("Path is /path2 OR /anno/path2")
			httpExp.GET("/other/path2").WithHost("www.example.com").Expect().
				Status(http.StatusNotFound)

			httpExp.GET("/path3").WithHost("www.example.com").WithHeader("HeaderName", "HeaderValue1").Expect().
				Status(http.StatusOK).
				Body().Equal("Http header HeaderName is HeaderValue1 OR HeaderValue2")
			httpExp.GET("/path3").WithHost("www.example.com").WithHeader("HeaderName", "HeaderValue2").Expect().
				Status(http.StatusOK).
				Body().Equal("Http header HeaderName is HeaderValue1 OR HeaderValue2")
			httpExp.GET("/path3").WithHost("www.example.com").WithHeader("HeaderName", "HeaderValue3").Expect().
				Status(http.StatusNotFound)
			httpExp.GET("/path3").WithHost("www.example.com").Expect().
				Status(http.StatusNotFound)

			httpExp.GET("/path4").WithHost("www.example.com").Expect().
				Status(http.StatusOK).
				Body().Equal("Http request method is GET OR HEAD")
			httpExp.HEAD("/path4").WithHost("www.example.com").Expect().
				Status(http.StatusOK)
			httpExp.POST("/path4").WithHost("www.example.com").Expect().
				Status(http.StatusNotFound)

			httpExp.GET("/path5").WithHost("www.example.com").WithQuery("paramA", "valueA1").Expect().
				Status(http.StatusOK).
				Body().Equal("Query string is paramA:valueA1 OR paramA:valueA2")
			httpExp.GET("/path5").WithHost("www.example.com").WithQuery("paramA", "valueA2").Expect().
				Status(http.StatusOK).
				Body().Equal("Query string is paramA:valueA1 OR paramA:valueA2")
			httpExp.GET("/path5").WithHost("www.example.com").WithQuery("paramA", "valueA3").Expect().
				Status(http.StatusNotFound)

			httpExp.GET("/path6").WithHost("www.example.com").Expect().
				Status(http.StatusNotFound)

			httpExp.GET("/path7").WithHost("www.example.com").
				WithHeader("HeaderName", "HeaderValue").
				WithQuery("paramA", "valueA").WithQuery("paramB", "valueB").Expect().
				Status(http.StatusOK).
				Body().Equal("multiple conditions applies")
			httpExp.GET("/path7").WithHost("www.example.com").
				WithHeader("HeaderName", "OtherHeaderValue").
				WithQuery("paramA", "valueA").WithQuery("paramB", "valueB").Expect().
				Status(http.StatusNotFound)
			httpExp.GET("/path7").WithHost("www.example.com").
				WithHeader("HeaderName", "HeaderValue").
				WithQuery("paramA", "valueB").WithQuery("paramB", "valueB").Expect().
				Status(http.StatusNotFound)
		})
	})
})

// ExpectOneLBProvisionedForIngress expects one LoadBalancer provisioned for Ingress.
func ExpectOneLBProvisionedForIngress(ctx context.Context, tf *framework.Framework, ing *networking.Ingress) (lbARN string, lbDNS string) {
	Eventually(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, k8s.NamespacedName(ing), ing)
		g.Expect(err).NotTo(HaveOccurred())
		lbDNS = FindIngressDNSName(ing)
		g.Expect(lbDNS).ShouldNot(BeEmpty())
	}, utils.IngressReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
	tf.Logger.Info("ingress DNS populated", "dnsName", lbDNS)

	var err error
	lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, lbDNS)
	Expect(err).ShouldNot(HaveOccurred())
	tf.Logger.Info("ALB provisioned", "arn", lbARN)
	return lbARN, lbDNS
}

// ExpectNoLBProvisionedForIngress expects no LoadBalancer provisioned for Ingress.
func ExpectNoLBProvisionedForIngress(ctx context.Context, tf *framework.Framework, ing *networking.Ingress) {
	Consistently(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, k8s.NamespacedName(ing), ing)
		g.Expect(err).NotTo(HaveOccurred())
		lbDNS := FindIngressDNSName(ing)
		g.Expect(lbDNS).Should(BeEmpty())
	}, utils.IngressReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
}

func ExpectLBDNSBeAvailable(ctx context.Context, tf *framework.Framework, lbARN string, lbDNS string) {
	ctx, cancel := context.WithTimeout(ctx, utils.IngressDNSAvailableWaitTimeout)
	defer cancel()

	tf.Logger.Info("wait loadBalancer becomes available", "arn", lbARN)
	err := tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())
	tf.Logger.Info("loadBalancer becomes available", "arn", lbARN)

	tf.Logger.Info("wait dns becomes available", "dns", lbDNS)
	err = utils.WaitUntilDNSNameAvailable(ctx, lbDNS)
	Expect(err).NotTo(HaveOccurred())
	tf.Logger.Info("dns becomes available", "dns", lbDNS)
}
