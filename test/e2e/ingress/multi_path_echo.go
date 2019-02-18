package ingress

import (
	"context"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/ingress/shared"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/onsi/ginkgo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MultiPathEchoStack struct {
	Deployment *appsv1.Deployment
	Service    *corev1.Service
	Ingress    *extensionsv1.Ingress
}

func NewMultiPathEchoStack(stackName string, modIP bool) *MultiPathEchoStack {
	stackReplicas := int32(3)
	stackLabels := map[string]string{
		"app.kubernetes.io/name": stackName,
	}
	dp := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: stackName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &stackReplicas,
			Selector: &metav1.LabelSelector{MatchLabels: stackLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: stackLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "echoserver",
							Image: "gcr.io/google_containers/echoserver:1.4",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	svcType := corev1.ServiceTypeNodePort
	if modIP {
		svcType = corev1.ServiceTypeClusterIP
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: stackName,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: stackLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "port1",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "port2",
					Port:       8443,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	ingTargetType := "instance"
	if modIP {
		ingTargetType = "ip"
	}
	ing := &extensionsv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: stackName,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class":                            "alb",
				"alb.ingress.kubernetes.io/scheme":                       "internet-facing",
				"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "5",
				"alb.ingress.kubernetes.io/healthcheck-timeout-seconds":  "2",
				"alb.ingress.kubernetes.io/target-type":                  ingTargetType,
			},
		},
		Spec: extensionsv1.IngressSpec{
			Rules: []extensionsv1.IngressRule{
				{
					IngressRuleValue: extensionsv1.IngressRuleValue{
						HTTP: &extensionsv1.HTTPIngressRuleValue{
							Paths: []extensionsv1.HTTPIngressPath{
								{
									Path: "/path1",
									Backend: extensionsv1.IngressBackend{
										ServiceName: stackName,
										ServicePort: intstr.FromInt(80),
									},
								},
								{
									Path: "/path2",
									Backend: extensionsv1.IngressBackend{
										ServiceName: stackName,
										ServicePort: intstr.FromInt(8443),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return &MultiPathEchoStack{
		Deployment: dp,
		Service:    svc,
		Ingress:    ing,
	}
}

func (s *MultiPathEchoStack) ExpectDeploySuccessfully(ctx context.Context, f *framework.Framework, ns *corev1.Namespace) {
	ginkgo.By("create deployment")
	dp, err := f.ClientSet.AppsV1().Deployments(ns.Name).Create(s.Deployment)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("create service")
	svc, err := f.ClientSet.CoreV1().Services(ns.Name).Create(s.Service)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("create ingress")
	ing, err := f.ClientSet.ExtensionsV1beta1().Ingresses(ns.Name).Create(s.Ingress)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("wait deployment")
	dp, err = f.ResourceManager.WaitDeploymentReady(ctx, dp)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("wait service")
	_, err = f.ResourceManager.WaitServiceHasEndpointsNum(ctx, svc, int(*dp.Spec.Replicas))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("wait ingress")
	ing, err = f.ResourceManager.WaitIngressReady(ctx, ing)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	utils.Logf("ingress DNS created: %v", ing.Status.LoadBalancer.Ingress[0].Hostname)

	awsRes, err := shared.GetAWSResourcesByIngress(f.Cloud, f.Options.ClusterName, ns.Name, ing.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	utils.Logf("ingress AWS Resources created: %v", awsRes)

	for _, tgArn := range awsRes.TargetGroups {
		ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		shared.ExpectTargetGroupTargetsEventuallyHealth(ctx, f.Cloud, tgArn)
		cancel()
	}
}

func (s *MultiPathEchoStack) ExpectCleanupSuccessfully(ctx context.Context, f *framework.Framework, ns *corev1.Namespace) {
	ginkgo.By("delete ingress")
	err := f.ClientSet.ExtensionsV1beta1().Ingresses(ns.Name).Delete(s.Ingress.Name, &metav1.DeleteOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("delete service")
	err = f.ClientSet.CoreV1().Services(ns.Name).Delete(s.Service.Name, &metav1.DeleteOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("delete deployment")
	err = f.ClientSet.AppsV1().Deployments(ns.Name).Delete(s.Deployment.Name, &metav1.DeleteOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	shared.ExpectAWSResourcedByIngressEventuallyDeleted(ctx, f.Cloud, f.Options.ClusterName, ns.Name, s.Ingress.Name)
	cancel()
}

var _ = ginkgo.Describe("Ingress with multi-path echo backend", func() {
	f := framework.New()

	var (
		ctx context.Context
		ns  *corev1.Namespace
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		var err error
		ns, err = f.ResourceManager.CreateNamespaceUnique(context.TODO(), "ingress")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("[mod-instance] should work", func() {
		stackName := "multi-path-echo"
		stack := NewMultiPathEchoStack(stackName, false)
		stack.ExpectDeploySuccessfully(ctx, f, ns)
		stack.ExpectCleanupSuccessfully(ctx, f, ns)
	})

	ginkgo.It("[mod-ip] should work", func() {
		stackName := "multi-path-echo"
		stack := NewMultiPathEchoStack(stackName, true)
		stack.ExpectDeploySuccessfully(ctx, f, ns)
		stack.ExpectCleanupSuccessfully(ctx, f, ns)
	})
})
