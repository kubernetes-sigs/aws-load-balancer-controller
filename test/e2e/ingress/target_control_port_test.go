package ingress

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/gavv/httpexpect/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	framework "sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/fixture"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/manifest"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	targetControlImage = "public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest"
)

var _ = Describe("ALB target control agent injection and target control port tests", func() {
	var (
		ctx       context.Context
		sandboxNS *corev1.Namespace
	)

	exact := networking.PathTypeExact

	BeforeEach(func() {
		ctx = context.Background()
		By("setup sandbox namespace", func() {
			tf.Logger.Info("allocating namespace")
			ns, err := tf.NSManager.AllocateNamespace(ctx, "alb-target-control-e2e")
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

	It("should inject ALB target control agent sidecar and set target control port when enabled", func() {
		if strings.Contains(tf.Options.AWSRegion, "-iso-") || tf.Options.AWSRegion == "eusc-de-east-1" {
			Skip("Skipping test, target control port not supported in this region")
		}
		dataAddress := "0.0.0.0:80"
		controlAddress := "0.0.0.0:3000"
		destinationAddress := "127.0.0.1:8080"

		if tf.Options.IPFamily == framework.IPv6 {
			dataAddress = "[::]:80"
			controlAddress = "[::]:3000"
			destinationAddress = "[::1]:8080"
		}

		albTargetControlConfig := &elbv2api.ALBTargetControlConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aws-load-balancer-controller-alb-target-control-agent-config",
				Namespace: sandboxNS.Name,
			},
			Spec: elbv2api.ALBTargetControlConfigSpec{
				Image:              targetControlImage,
				DataAddress:        dataAddress,
				ControlAddress:     controlAddress,
				DestinationAddress: destinationAddress,
				MaxConcurrency:     1000,
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		}

		err := tf.K8sClient.Create(ctx, albTargetControlConfig)
		if err != nil {
			if apierrs.IsNotFound(err) || meta.IsNoMatchError(err) {
				Skip("Skipping test, ALBTargetControlConfig CRD not available")
			}
			if !apierrs.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		}

		sandboxNS.Labels = map[string]string{
			"elbv2.k8s.aws/alb-target-control-agent-injection": "enabled",
		}
		err = tf.K8sClient.Update(ctx, sandboxNS)
		Expect(err).NotTo(HaveOccurred())

		appBuilder := manifest.NewFixedResponseServiceBuilder()
		ingBuilder := manifest.NewIngressBuilder()
		dp, svc := appBuilder.Build(sandboxNS.Name, "app", tf.Options.TestImageRegistry)

		// Update service to use port 80 for ALB target control agent traffic
		svc.Spec.Ports[0].Port = 80
		svc.Spec.Ports[0].TargetPort = intstr.FromInt(80)

		// Update deployment to use port 8080 for application listener
		dp.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = 8080

		ingBackend := networking.IngressBackend{
			Service: &networking.IngressServiceBackend{
				Name: svc.Name,
				Port: networking.ServiceBackendPort{
					Number: 80,
				},
			},
		}

		annotation := map[string]string{
			"kubernetes.io/ingress.class":                                                "alb",
			"alb.ingress.kubernetes.io/scheme":                                           "internet-facing",
			"alb.ingress.kubernetes.io/target-type":                                      "ip",
			"alb.ingress.kubernetes.io/listen-ports":                                     "[{\"HTTP\": 80}]",
			fmt.Sprintf("alb.ingress.kubernetes.io/target-control-port.%s.80", svc.Name): "3000",
		}
		if tf.Options.IPFamily == framework.IPv6 {
			annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
		}

		ing := ingBuilder.
			AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
			WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
		resStack := fixture.NewK8SResourceStack(tf, dp, svc, ing)
		err = resStack.Setup(ctx)
		Expect(err).NotTo(HaveOccurred())

		lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)
		// Check target group has control port set
		Eventually(func() bool {
			tgs, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
			if err != nil {
				tf.Logger.Info("Failed to get target groups", "error", err)
				return false
			}
			for _, tg := range tgs {
				if tg.TargetControlPort != nil && awssdk.ToInt32(tg.TargetControlPort) == 3000 {
					tf.Logger.Info("Found target group with control port", "port", awssdk.ToInt32(tg.TargetControlPort))
					return true
				}
			}
			tf.Logger.Info("No target group with control port 3000 found", "count", len(tgs))
			return false
		}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

		// Check ALB target control agent sidecar injection
		Eventually(func() bool {
			pods := &corev1.PodList{}
			err := tf.K8sClient.List(ctx, pods, &client.ListOptions{Namespace: sandboxNS.Name})
			if err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, container := range pod.Spec.Containers {
					if container.Name == "alb-target-control-agent" {
						tf.Logger.Info("Found ALB target control agent sidecar", "pod", pod.Name, "image", container.Image)
						return true
					}
				}
			}
			tf.Logger.Info("ALB target control agent sidecar not found", "podCount", len(pods.Items))
			return false
		}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

		// Test HTTP traffic through the load balancer
		ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
		httpExp := httpexpect.New(tf.LoggerReporter, fmt.Sprintf("http://%v", lbDNS))
		httpExp.GET("/path").Expect().
			Status(http.StatusOK).
			Body().Equal("Hello World!")
	})

	It("should set target control port with manual sidecar injection", func() {
		if strings.Contains(tf.Options.AWSRegion, "-iso-") || tf.Options.AWSRegion == "eusc-de-east-1" {
			Skip("Skipping test, target control port not supported in this region")
		}
		dataAddress := "0.0.0.0:80"
		controlAddress := "0.0.0.0:4000"
		destinationAddress := "127.0.0.1:8080"

		if tf.Options.IPFamily == framework.IPv6 {
			dataAddress = "[::]:80"
			controlAddress = "[::]:4000"
			destinationAddress = "[::1]:8080"
		}

		appBuilder := manifest.NewFixedResponseServiceBuilder()
		ingBuilder := manifest.NewIngressBuilder()
		dp, svc := appBuilder.Build(sandboxNS.Name, "app", tf.Options.TestImageRegistry)

		svc.Spec.Ports[0].Port = 80
		svc.Spec.Ports[0].TargetPort = intstr.FromInt(80)
		dp.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = 8080

		// Manually inject target control agent as sidecar
		dp.Spec.Template.Spec.Containers = append(dp.Spec.Template.Spec.Containers, corev1.Container{
			Name:            "alb-target-control-agent",
			Image:           targetControlImage,
			ImagePullPolicy: corev1.PullAlways,
			Env: []corev1.EnvVar{
				{Name: "TARGET_CONTROL_DATA_ADDRESS", Value: dataAddress},
				{Name: "TARGET_CONTROL_CONTROL_ADDRESS", Value: controlAddress},
				{Name: "TARGET_CONTROL_DESTINATION", Value: destinationAddress},
				{Name: "TARGET_CONTROL_MAX_CONCURRENCY", Value: "1000"},
				{Name: "RUST_LOG", Value: "debug"},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
		})

		ingBackend := networking.IngressBackend{
			Service: &networking.IngressServiceBackend{
				Name: svc.Name,
				Port: networking.ServiceBackendPort{
					Number: 80,
				},
			},
		}

		annotation := map[string]string{
			"kubernetes.io/ingress.class":                                                "alb",
			"alb.ingress.kubernetes.io/scheme":                                           "internet-facing",
			"alb.ingress.kubernetes.io/target-type":                                      "ip",
			"alb.ingress.kubernetes.io/listen-ports":                                     "[{\"HTTP\": 80}]",
			fmt.Sprintf("alb.ingress.kubernetes.io/target-control-port.%s.80", svc.Name): "4000",
		}
		if tf.Options.IPFamily == framework.IPv6 {
			annotation["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
		}

		ing := ingBuilder.
			AddHTTPRoute("", networking.HTTPIngressPath{Path: "/path", PathType: &exact, Backend: ingBackend}).
			WithAnnotations(annotation).Build(sandboxNS.Name, "ing")
		resStack := fixture.NewK8SResourceStack(tf, dp, svc, ing)
		err := resStack.Setup(ctx)
		Expect(err).NotTo(HaveOccurred())

		lbARN, lbDNS := ExpectOneLBProvisionedForIngress(ctx, tf, ing)

		// Check target group has control port set to 4000
		Eventually(func() bool {
			tgs, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
			if err != nil {
				return false
			}
			for _, tg := range tgs {
				if tg.TargetControlPort != nil && awssdk.ToInt32(tg.TargetControlPort) == 4000 {
					return true
				}
			}
			return false
		}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

		// Test HTTP traffic
		ExpectLBDNSBeAvailable(ctx, tf, lbARN, lbDNS)
		httpExp := httpexpect.New(tf.LoggerReporter, fmt.Sprintf("http://%v", lbDNS))
		httpExp.GET("/path").Expect().
			Status(http.StatusOK).
			Body().Equal("Hello World!")
	})
})
