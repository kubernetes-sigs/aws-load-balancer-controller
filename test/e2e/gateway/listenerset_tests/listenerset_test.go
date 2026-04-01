package listenerset_tests

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
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/test_resources"
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
			err := test_resources.DeleteNamespace(ctx, tf, ns)
			Expect(err).NotTo(HaveOccurred())
		}
		if gwc != nil {
			err := test_resources.DeleteGatewayClass(ctx, tf, gwc)
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
				ns, err = test_resources.AllocateNamespace(ctx, tf, "ls-e2e", map[string]string{})
				Expect(err).NotTo(HaveOccurred())

				gwc = test_resources.BuildGatewayClassSpec("gateway.k8s.aws/alb")
				err = test_resources.CreateGatewayClass(ctx, tf, gwc)
				Expect(err).NotTo(HaveOccurred())
			})

			var dp *appsv1.Deployment
			var svc *corev1.Service

			By("creating deployment and service", func() {
				dp = test_resources.BuildDeploymentSpec(tf.Options.TestImageRegistry)
				dp.Namespace = ns.Name
				svc = test_resources.BuildServiceSpec(map[string]string{})
				svc.Namespace = ns.Name

				err := test_resources.CreateDeployments(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
				err = test_resources.CreateServices(ctx, tf, []*corev1.Service{svc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating LB config and TG config", func() {
				lbc := test_resources.BuildLoadBalancerConfig(lbcSpec)
				lbc.Namespace = ns.Name
				err := test_resources.CreateLoadBalancerConfig(ctx, tf, lbc)
				Expect(err).NotTo(HaveOccurred())

				tgc := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgSpec, svc)
				tgc.Namespace = ns.Name
				err = test_resources.CreateTargetGroupConfigs(ctx, tf, []*elbv2gw.TargetGroupConfiguration{tgc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating gateway with AllowedListeners from same namespace", func() {
				gw = test_resources.BuildBasicGatewaySpec(gwc, gwListeners)
				gw.Namespace = ns.Name
				gw.Spec.AllowedListeners = &gwv1.AllowedListeners{
					Namespaces: &gwv1.ListenerNamespaces{From: &nsSame},
				}
				err := test_resources.CreateGateway(ctx, tf, gw)
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
				httpr := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("gw-http")))
				httpr.Namespace = ns.Name
				tf.Logger.Info("creating http route for gw listener", "httpr", k8s.NamespacedName(httpr))
				err := tf.K8sClient.Create(ctx, httpr)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the ListenerSet listener", func() {
				httprLS := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("ls-http")))
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
				err := test_resources.WaitUntilDeploymentReady(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
			})

			var dnsName string
			By("waiting for gateway to be programmed", func() {
				observedGW, err := test_resources.WaitUntilGatewayReady(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(observedGW.Status.Addresses).NotTo(BeEmpty())
				dnsName = observedGW.Status.Addresses[0].Value
				Expect(dnsName).NotTo(BeEmpty())
				Expect(*observedGW.Status.AttachedListenerSets).To(Equal(int32(1)))
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
							TargetGroupHC: test_resources.DEFAULT_ALB_TARGET_GROUP_HC,
						},
						{
							Protocol:      "HTTP",
							Port:          80,
							NumTargets:    int(*dp.Spec.Replicas),
							TargetType:    "ip",
							TargetGroupHC: test_resources.DEFAULT_ALB_TARGET_GROUP_HC,
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

	Context("with Gateway that does not allow ListenerSets", func() {
		It("should reject the ListenerSet and not create its listener", func() {
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
				ns, err = test_resources.AllocateNamespace(ctx, tf, "ls-reject-e2e", map[string]string{})
				Expect(err).NotTo(HaveOccurred())

				gwc = test_resources.BuildGatewayClassSpec("gateway.k8s.aws/alb")
				err = test_resources.CreateGatewayClass(ctx, tf, gwc)
				Expect(err).NotTo(HaveOccurred())
			})

			var dp *appsv1.Deployment
			var svc *corev1.Service

			By("creating deployment and service", func() {
				dp = test_resources.BuildDeploymentSpec(tf.Options.TestImageRegistry)
				dp.Namespace = ns.Name
				svc = test_resources.BuildServiceSpec(map[string]string{})
				svc.Namespace = ns.Name

				err := test_resources.CreateDeployments(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
				err = test_resources.CreateServices(ctx, tf, []*corev1.Service{svc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating LB config and TG config", func() {
				lbc := test_resources.BuildLoadBalancerConfig(lbcSpec)
				lbc.Namespace = ns.Name
				err := test_resources.CreateLoadBalancerConfig(ctx, tf, lbc)
				Expect(err).NotTo(HaveOccurred())

				tgc := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgSpec, svc)
				tgc.Namespace = ns.Name
				err = test_resources.CreateTargetGroupConfigs(ctx, tf, []*elbv2gw.TargetGroupConfiguration{tgc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating gateway without AllowedListeners (defaults to None)", func() {
				gw = test_resources.BuildBasicGatewaySpec(gwc, gwListeners)
				gw.Namespace = ns.Name
				// No AllowedListeners set — defaults to NamespacesFromNone
				err := test_resources.CreateGateway(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating ListenerSet in the same namespace", func() {
				ls := &gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rejected-listenerset",
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
				httpr := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("gw-http")))
				httpr.Namespace = ns.Name
				err := tf.K8sClient.Create(ctx, httpr)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the ListenerSet listener", func() {
				httprLS := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("ls-http")))
				httprLS.Namespace = ns.Name
				httprLS.Spec.ParentRefs = []gwv1.ParentReference{
					{
						Name:        "rejected-listenerset",
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
				err := test_resources.WaitUntilDeploymentReady(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
			})

			var dnsName string
			By("waiting for gateway to be programmed", func() {
				observedGW, err := test_resources.WaitUntilGatewayReady(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(observedGW.Status.Addresses).NotTo(BeEmpty())
				dnsName = observedGW.Status.Addresses[0].Value
				Expect(dnsName).NotTo(BeEmpty())
				Expect(*observedGW.Status.AttachedListenerSets).To(Equal(int32(0)))
			})

			var lbARN string
			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).NotTo(BeEmpty())
			})

			By("verifying AWS loadbalancer only has the gateway listener, not the ListenerSet listener", func() {
				err := verifier.VerifyLoadBalancerListeners(ctx, tf, lbARN, map[string]string{
					"80": "HTTP",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying ListenerSet status is rejected with NotAllowed reason", func() {
				time.Sleep(10 * time.Second)
				ls := &gwv1.ListenerSet{}
				err := tf.K8sClient.Get(ctx, k8s.NamespacedName(&gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{Name: "rejected-listenerset", Namespace: ns.Name},
				}), ls)
				Expect(err).NotTo(HaveOccurred())

				var acceptedFound bool
				for _, cond := range ls.Status.Conditions {
					if cond.Type == string(gwv1.ListenerSetConditionAccepted) {
						acceptedFound = true
						Expect(string(cond.Status)).To(Equal("False"))
						Expect(cond.Reason).To(Equal(string(gwv1.ListenerSetReasonNotAllowed)))
					}
				}
				Expect(acceptedFound).To(BeTrue(), "ListenerSet should have Accepted=False condition")
			})

			By("sending http request to the gateway listener port", func() {
				url := fmt.Sprintf("http://%v/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with Gateway allowing ListenerSets from all namespaces", func() {
		var lsNs *corev1.Namespace

		AfterEach(func() {
			if lsNs != nil {
				err := test_resources.DeleteNamespace(ctx, tf, lsNs)
				Expect(err).NotTo(HaveOccurred())
				lsNs = nil
			}
		})

		It("should accept a ListenerSet from a different namespace", func() {
			nsAll := gwv1.NamespacesFromAll

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

			By("setting up gateway namespace and base resources", func() {
				var err error
				ns, err = test_resources.AllocateNamespace(ctx, tf, "ls-allns-gw", map[string]string{})
				Expect(err).NotTo(HaveOccurred())

				gwc = test_resources.BuildGatewayClassSpec("gateway.k8s.aws/alb")
				err = test_resources.CreateGatewayClass(ctx, tf, gwc)
				Expect(err).NotTo(HaveOccurred())
			})

			var dp *appsv1.Deployment
			var svc *corev1.Service

			By("creating deployment and service", func() {
				dp = test_resources.BuildDeploymentSpec(tf.Options.TestImageRegistry)
				dp.Namespace = ns.Name
				svc = test_resources.BuildServiceSpec(map[string]string{})
				svc.Namespace = ns.Name

				err := test_resources.CreateDeployments(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
				err = test_resources.CreateServices(ctx, tf, []*corev1.Service{svc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating LB config and TG config", func() {
				lbc := test_resources.BuildLoadBalancerConfig(lbcSpec)
				lbc.Namespace = ns.Name
				err := test_resources.CreateLoadBalancerConfig(ctx, tf, lbc)
				Expect(err).NotTo(HaveOccurred())

				tgc := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgSpec, svc)
				tgc.Namespace = ns.Name
				err = test_resources.CreateTargetGroupConfigs(ctx, tf, []*elbv2gw.TargetGroupConfiguration{tgc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating gateway with AllowedListeners from all namespaces", func() {
				gw = test_resources.BuildBasicGatewaySpec(gwc, gwListeners)
				gw.Namespace = ns.Name
				gw.Spec.AllowedListeners = &gwv1.AllowedListeners{
					Namespaces: &gwv1.ListenerNamespaces{From: &nsAll},
				}
				err := test_resources.CreateGateway(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating a separate namespace for the ListenerSet", func() {
				var err error
				lsNs, err = test_resources.AllocateNamespace(ctx, tf, "ls-allns-ls", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating ListenerSet in the different namespace", func() {
				gwNs := gwv1.Namespace(ns.Name)
				ls := &gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cross-ns-listenerset",
						Namespace: lsNs.Name,
					},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name:      gwv1.ObjectName(gw.Name),
							Namespace: &gwNs,
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
				tf.Logger.Info("creating cross-ns listener set", "ls", k8s.NamespacedName(ls))
				err := tf.K8sClient.Create(ctx, ls)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the gateway listener", func() {
				httpr := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("gw-http")))
				httpr.Namespace = ns.Name
				err := tf.K8sClient.Create(ctx, httpr)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the ListenerSet listener", func() {
				httprLS := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("ls-http")))
				httprLS.Namespace = lsNs.Name
				httprLS.Spec.ParentRefs = []gwv1.ParentReference{
					{
						Name:        "cross-ns-listenerset",
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
				err := test_resources.WaitUntilDeploymentReady(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
			})

			var dnsName string
			By("waiting for gateway to be programmed", func() {
				observedGW, err := test_resources.WaitUntilGatewayReady(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(observedGW.Status.Addresses).NotTo(BeEmpty())
				dnsName = observedGW.Status.Addresses[0].Value
				Expect(dnsName).NotTo(BeEmpty())
				Expect(*observedGW.Status.AttachedListenerSets).To(Equal(int32(1)))
			})

			var lbARN string
			By("querying AWS loadbalancer from the dns name", func() {
				time.Sleep(10 * time.Minute)
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).NotTo(BeEmpty())
			})

			By("verifying AWS loadbalancer has listeners on both ports", func() {
				err := verifier.VerifyLoadBalancerListeners(ctx, tf, lbARN, map[string]string{
					"80":   "HTTP",
					"8080": "HTTP",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying cross-namespace ListenerSet status is accepted", func() {
				time.Sleep(10 * time.Second)
				ls := &gwv1.ListenerSet{}
				err := tf.K8sClient.Get(ctx, k8s.NamespacedName(&gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{Name: "cross-ns-listenerset", Namespace: lsNs.Name},
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

			By("sending http request to the gateway listener port", func() {
				url := fmt.Sprintf("http://%v/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("with Gateway allowing ListenerSets via namespace selector", func() {
		var lsNs *corev1.Namespace

		AfterEach(func() {
			if lsNs != nil {
				err := test_resources.DeleteNamespace(ctx, tf, lsNs)
				Expect(err).NotTo(HaveOccurred())
				lsNs = nil
			}
		})

		It("should accept a ListenerSet from a namespace matching the selector", func() {
			nsSelector := gwv1.NamespacesFromSelector

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

			By("setting up gateway namespace and base resources", func() {
				var err error
				ns, err = test_resources.AllocateNamespace(ctx, tf, "ls-sel-gw", map[string]string{})
				Expect(err).NotTo(HaveOccurred())

				gwc = test_resources.BuildGatewayClassSpec("gateway.k8s.aws/alb")
				err = test_resources.CreateGatewayClass(ctx, tf, gwc)
				Expect(err).NotTo(HaveOccurred())
			})

			var dp *appsv1.Deployment
			var svc *corev1.Service

			var lsdp *appsv1.Deployment
			var lssvc *corev1.Service

			By("creating a labeled namespace for the ListenerSet", func() {
				var err error
				lsNs, err = test_resources.AllocateNamespace(ctx, tf, "ls-sel-ls", map[string]string{
					"listenerset-allowed": "true",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating backend resources in gateway namespace", func() {
				dp = test_resources.BuildDeploymentSpec(tf.Options.TestImageRegistry)
				dp.Namespace = ns.Name
				svc = test_resources.BuildServiceSpec(map[string]string{})
				svc.Namespace = ns.Name

				err := test_resources.CreateDeployments(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
				err = test_resources.CreateServices(ctx, tf, []*corev1.Service{svc})
				Expect(err).NotTo(HaveOccurred())

				lbc := test_resources.BuildLoadBalancerConfig(lbcSpec)
				lbc.Namespace = ns.Name
				err = test_resources.CreateLoadBalancerConfig(ctx, tf, lbc)
				Expect(err).NotTo(HaveOccurred())

				tgc := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgSpec, svc)
				tgc.Namespace = ns.Name
				err = test_resources.CreateTargetGroupConfigs(ctx, tf, []*elbv2gw.TargetGroupConfiguration{tgc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating backend resources in listenerset namespace", func() {
				lsdp = test_resources.BuildDeploymentSpec(tf.Options.TestImageRegistry)
				lsdp.Namespace = lsNs.Name
				lssvc = test_resources.BuildServiceSpec(map[string]string{})
				lssvc.Namespace = lsNs.Name

				err := test_resources.CreateDeployments(ctx, tf, []*appsv1.Deployment{lsdp})
				Expect(err).NotTo(HaveOccurred())
				err = test_resources.CreateServices(ctx, tf, []*corev1.Service{lssvc})
				Expect(err).NotTo(HaveOccurred())

				lstgc := test_resources.BuildTargetGroupConfig(test_resources.DefaultTgConfigName, tgSpec, lssvc)
				lstgc.Namespace = lsNs.Name
				err = test_resources.CreateTargetGroupConfigs(ctx, tf, []*elbv2gw.TargetGroupConfiguration{lstgc})
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating gateway with AllowedListeners using namespace selector", func() {
				gw = test_resources.BuildBasicGatewaySpec(gwc, gwListeners)
				gw.Namespace = ns.Name
				gw.Spec.AllowedListeners = &gwv1.AllowedListeners{
					Namespaces: &gwv1.ListenerNamespaces{
						From: &nsSelector,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"listenerset-allowed": "true",
							},
						},
					},
				}
				err := test_resources.CreateGateway(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating ListenerSet in the labeled namespace", func() {
				gwNs := gwv1.Namespace(ns.Name)
				ls := &gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "selector-listenerset",
						Namespace: lsNs.Name,
					},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name:      gwv1.ObjectName(gw.Name),
							Namespace: &gwNs,
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
				tf.Logger.Info("creating selector-based listener set", "ls", k8s.NamespacedName(ls))
				err := tf.K8sClient.Create(ctx, ls)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the gateway listener", func() {
				httpr := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("gw-http")))
				httpr.Namespace = ns.Name
				err := tf.K8sClient.Create(ctx, httpr)
				Expect(err).NotTo(HaveOccurred())
			})

			By("creating HTTPRoute targeting the ListenerSet listener", func() {
				httprLS := test_resources.BuildHTTPRoute([]string{}, []gwv1.HTTPRouteRule{}, new(gwv1.SectionName("ls-http")))
				httprLS.Namespace = lsNs.Name
				httprLS.Spec.ParentRefs = []gwv1.ParentReference{
					{
						Name:        "selector-listenerset",
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
				err := test_resources.WaitUntilDeploymentReady(ctx, tf, []*appsv1.Deployment{dp})
				Expect(err).NotTo(HaveOccurred())
			})

			var dnsName string
			By("waiting for gateway to be programmed", func() {
				observedGW, err := test_resources.WaitUntilGatewayReady(ctx, tf, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(observedGW.Status.Addresses).NotTo(BeEmpty())
				Expect(*observedGW.Status.AttachedListenerSets).To(Equal(int32(1)))
				dnsName = observedGW.Status.Addresses[0].Value
				Expect(dnsName).NotTo(BeEmpty())
				time.Sleep(10 * time.Minute)
			})

			var lbARN string
			By("querying AWS loadbalancer from the dns name", func() {
				var err error
				lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
				Expect(lbARN).NotTo(BeEmpty())
			})

			By("verifying AWS loadbalancer has listeners on both ports", func() {
				err := verifier.VerifyLoadBalancerListeners(ctx, tf, lbARN, map[string]string{
					"80":   "HTTP",
					"8080": "HTTP",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying selector-matched ListenerSet status is accepted", func() {
				time.Sleep(10 * time.Second)
				ls := &gwv1.ListenerSet{}
				err := tf.K8sClient.Get(ctx, k8s.NamespacedName(&gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{Name: "selector-listenerset", Namespace: lsNs.Name},
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
		})
	})
})
