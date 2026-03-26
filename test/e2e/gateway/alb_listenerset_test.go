package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("test k8s alb gateway with ListenerSet", func() {
	var (
		ctx context.Context
		ns  *corev1.Namespace
		gwc *gwv1.GatewayClass
		gw  *gwv1.Gateway
	)

	BeforeEach(func() {
		if !tf.Options.EnableGatewayTests {
			Skip("Skipping gateway tests")
		}
		ctx = context.Background()
	})

	AfterEach(func() {
		if ns != nil {
			err := deleteNamespace(ctx, tf, ns)
			Expect(err).NotTo(HaveOccurred())
		}
		if gwc != nil {
			err := deleteGatewayClass(ctx, tf, gwc)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("with ALB ip target and ListenerSet providing additional listener", func() {
		It("should provision load balancer with listeners from both Gateway and ListenerSet", func() {
			nsSame := gwv1.NamespacesFromSame

			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}
			ipTargetType := elbv2gw.TargetTypeIP
			tgSpec := elbv2gw.TargetGroupConfigurationSpec{
				DefaultConfiguration: elbv2gw.TargetGroupProps{
					TargetType: &ipTargetType,
				},
			}

			gwListeners := []gwv1.Listener{
				{
					Name:     "gw-http",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
			}

			By("setting up namespace and base resources", func() {
				var err error
				ns, err = allocateNamespace(ctx, tf, "ls-e2e", map[string]string{})
				Expect(err).NotTo(HaveOccurred())

				gwc = buildGatewayClassSpec("gateway.k8s.aws/alb")
				err = createGatewayClass(ctx, tf, gwc)
				Expect(err).NotTo(HaveOccurred())
			})

			var dp *appsv1.Deployment
			var svc *corev1.Service

			By("creating deployment and service", func() {
				dp = buildDeploymentSpec(tf.Options.TestImageRegistry)
				dp.Namespace = ns.Name
				svc = buildServiceSpec(map[string]string{})
				svc.Namespace = ns.Name

				err := createDeployments(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
				err = createServices(ctx, tf, []*corev1.Service{svc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating LB config and TG config", func() {
				lbc := buildLoadBalancerConfig(lbcSpec)
				lbc.Namespace = ns.Name
				err := createLoadBalancerConfig(ctx, tf, lbc)
				Expect(err).NotTo(HaveOccurred())

				tgc := buildTargetGroupConfig(defaultTgConfigName, tgSpec, svc)
				tgc.Namespace = ns.Name
				err = createTargetGroupConfigs(ctx, tf, []*elbv2gw.TargetGroupConfiguration{tgc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating gateway with AllowedListeners from same namespace", func() {
				gw = buildBasicGatewaySpec(gwc, gwListeners)
				gw.Namespace = ns.Name
				gw.Spec.AllowedListeners = &gwv1.AllowedListeners{
					Namespaces: &gwv1.ListenerNamespaces{From: &nsSame},
				}
				err := createGateway(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating ListenerSet in the same namespace", func() {
				ls := &gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-listenerset",
						Namespace: ns.Name,
					},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name: gwv1.ObjectName(gw.Name),
						},
						Listeners: []gwv1.ListenerEntry{
							{
								Name:     "ls-http",
								Port:     8080,
								Protocol: gwv1.HTTPProtocolType,
							},
						},
					},
				}
				tf.Logger.Info("creating listener set", "ls", k8s.NamespacedName(ls))
				err := tf.K8sClient.Create(ctx, ls)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the gateway listener", func() {
				httpr := BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("gw-http")))
				httpr.Namespace = ns.Name
				tf.Logger.Info("creating http route for gw listener", "httpr", k8s.NamespacedName(httpr))
				err := tf.K8sClient.Create(ctx, httpr)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the ListenerSet listener", func() {
				httprLS := BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("ls-http")))
				httprLS.Namespace = ns.Name
				httprLS.Spec.ParentRefs = []gwv1.ParentReference{
					{
						Name:        "test-listenerset",
						Kind:        (*gwv1.Kind)(new("ListenerSet")),
						Group:       (*gwv1.Group)(new("gateway.networking.k8s.io")),
						SectionName: (*gwv1.SectionName)(new("ls-http")),
					},
				}
				tf.Logger.Info("creating http route for ls listener", "httpr", k8s.NamespacedName(httprLS))
				err := tf.K8sClient.Create(ctx, httprLS)
				Expect(err).NotTo(HaveOccurred())
			})

			By("waiting for deployment to be ready", func() {
				err := waitUntilDeploymentReady(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
			})

			var dnsName string
			By("waiting for gateway to be programmed", func() {
				observedGW, err := waitUntilGatewayReady(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(observedGW.Status.Addresses).NotTo(BeEmpty())
				dnsName = observedGW.Status.Addresses[0].Value
				Expect(dnsName).NotTo(BeEmpty())
			})

			var lbARN string
			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).NotTo(BeEmpty())
			})

			By("verifying AWS loadbalancer has listeners on both ports", func() {
				err := verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:   "application",
					Scheme: "internet-facing",
					Listeners: map[string]string{
						"80":   "HTTP",
						"8080": "HTTP",
					},
					TargetGroups: []verifier.ExpectedTargetGroup{
						{
							Protocol:      "HTTP",
							Port:          80,
							NumTargets:    int(*dp.Spec.Replicas),
							TargetType:    "ip",
							TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
						},
						{
							Protocol:      "HTTP",
							Port:          80,
							NumTargets:    int(*dp.Spec.Replicas),
							TargetType:    "ip",
							TargetGroupHC: DEFAULT_ALB_TARGET_GROUP_HC,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("waiting for target group targets to be healthy", func() {
				err := verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, int(*dp.Spec.Replicas))
				Expect(err).NotTo(HaveOccurred())
			})

			By("waiting until DNS name is available", func() {
				err := utils.WaitUntilDNSNameAvailable(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
			})

			By("sending http request to the gateway listener port", func() {
				url := fmt.Sprintf("http://%v/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})

			By("sending http request to the ListenerSet listener port", func() {
				url := fmt.Sprintf("http://%v:8080/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying ListenerSet status is accepted", func() {
				// Give some time for status reconciliation
				time.Sleep(10 * time.Second)
				ls := &gwv1.ListenerSet{}
				err := tf.K8sClient.Get(ctx, k8s.NamespacedName(&gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{Name: "test-listenerset", Namespace: ns.Name},
				}), ls)
				Expect(err).NotTo(HaveOccurred())

				var acceptedFound bool
				for _, cond := range ls.Status.Conditions {
					if cond.Type == string(gwv1.ListenerSetConditionAccepted) {
						acceptedFound = true
						Expect(string(cond.Status)).To(Equal("True"))
					}
				}
				Expect(acceptedFound).To(BeTrue(), "ListenerSet should have Accepted condition")
			})
		})
	})
})
