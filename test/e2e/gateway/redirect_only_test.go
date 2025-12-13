package gateway

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("HTTPRoute Redirect-Only Rules", func() {
	var (
		ctx         context.Context
		stack       *albResourceStack
		gwListeners []gwv1.Listener
		lbcSpec     *elbv2gw.LoadBalancerConfiguration
		tgSpec      *elbv2gw.TargetGroupConfiguration
	)

	BeforeEach(func() {
		ctx = context.Background()
		
		// Setup basic gateway listeners
		gwListeners = []gwv1.Listener{
			{
				Name:     "http",
				Port:     80,
				Protocol: gwv1.HTTPProtocolType,
			},
			{
				Name:     "https",
				Port:     443,
				Protocol: gwv1.HTTPSProtocolType,
			},
		}

		// Setup load balancer configuration
		lbcSpec = &elbv2gw.LoadBalancerConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultLbConfigName,
				Namespace: k8sClient.Namespace(),
			},
			Spec: elbv2gw.LoadBalancerConfigurationSpec{
				LoadBalancerName: awssdk.String("redirect-only-test-alb"),
			},
		}

		// Setup target group configuration (for backend rules)
		tgSpec = &elbv2gw.TargetGroupConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultTgConfigName,
				Namespace: k8sClient.Namespace(),
			},
			Spec: elbv2gw.TargetGroupConfigurationSpec{
				TargetReference: elbv2gw.TargetReference{
					Name: defaultName,
				},
			},
		}
	})

	Context("Redirect-Only HTTPRoute Rules", func() {
		It("should create ALB listener rules without target groups for redirect-only rules", func() {
			// Create HTTPRoute with redirect-only rules
			httpRouteRules := []gwv1.HTTPRouteRule{
				{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
								Value: awssdk.String("/redirect-to-https"),
							},
						},
					},
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Scheme:     awssdk.String("https"),
								StatusCode: awssdk.Int(301),
							},
						},
					},
					// No BackendRefs - this is redirect-only
				},
				{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
								Value: awssdk.String("/old-site"),
							},
						},
					},
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Hostname:   (*gwv1.PreciseHostname)(awssdk.String("new-site.example.com")),
								StatusCode: awssdk.Int(302),
							},
						},
					},
					// No BackendRefs - this is redirect-only
				},
			}

			httpr := buildHTTPRoute([]string{}, httpRouteRules, nil)

			By("deploying stack with redirect-only HTTPRoute", func() {
				stack = newALBResourceStack(
					nil,                    // No deployments needed for redirect-only
					nil,                    // No services needed for redirect-only
					buildGatewayClass(),
					buildGateway(gwListeners),
					lbcSpec,
					nil,                    // No target group configs needed
					nil,                    // No listener rule configs
					[]*gwv1.HTTPRoute{httpr},
					nil,                    // No GRPC routes
					nil,                    // No OIDC secret
					"redirect-only-test",
					false,                  // No pod readiness gate needed
				)

				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, nil, nil, nil, false)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying ALB is created", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyALBExists(ctx)
				}, 2*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("verifying listener rules are created for redirect-only rules", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyListenerRulesCount(ctx, 80, 2) // Should have 2 redirect rules
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("verifying no target groups are created for redirect-only rules", func() {
				Consistently(func() error {
					// Verify that no target groups exist for this load balancer
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyTargetGroupCount(ctx, 0) // Should have 0 target groups
				}, 30*time.Second, 5*time.Second).Should(Succeed())
			})

			By("verifying redirect functionality works", func() {
				// Test the redirect behavior (if possible in test environment)
				// This would require actual HTTP requests to the ALB
				Skip("Redirect functionality testing requires live ALB endpoint")
			})

			By("cleaning up resources", func() {
				err := stack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("should handle mixed rules (redirect-only and backend rules) correctly", func() {
			// Create deployment and service for backend rules
			deployment := buildDeployment(defaultName, defaultNumReplicas, appContainerPort)
			service := buildService(defaultName, appContainerPort, corev1.ServiceTypeClusterIP)

			// Create HTTPRoute with mixed rules
			httpRouteRules := []gwv1.HTTPRouteRule{
				{
					// Redirect-only rule
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
								Value: awssdk.String("/redirect"),
							},
						},
					},
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Scheme:     awssdk.String("https"),
								StatusCode: awssdk.Int(301),
							},
						},
					},
					// No BackendRefs
				},
				{
					// Backend rule
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
								Value: awssdk.String("/app"),
							},
						},
					},
					BackendRefs: DefaultHttpRouteRuleBackendRefs,
					// No redirect filters
				},
				{
					// Mixed rule (both redirect and backend)
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
								Value: awssdk.String("/mixed"),
							},
						},
					},
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Port: (*gwv1.PortNumber)(awssdk.Int32(443)),
							},
						},
					},
					BackendRefs: DefaultHttpRouteRuleBackendRefs,
				},
			}

			httpr := buildHTTPRoute([]string{}, httpRouteRules, nil)

			By("deploying stack with mixed HTTPRoute rules", func() {
				stack = newALBResourceStack(
					[]*appsv1.Deployment{deployment},
					[]*corev1.Service{service},
					buildGatewayClass(),
					buildGateway(gwListeners),
					lbcSpec,
					[]*elbv2gw.TargetGroupConfiguration{tgSpec},
					nil,
					[]*gwv1.HTTPRoute{httpr},
					nil,
					nil,
					"mixed-rules-test",
					true, // Enable pod readiness gate for backend rules
				)

				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, tgSpec, nil, nil, true)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying ALB is created", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyALBExists(ctx)
				}, 2*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("verifying correct number of listener rules", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyListenerRulesCount(ctx, 80, 3) // Should have 3 rules total
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("verifying target groups are created only for backend rules", func() {
				Eventually(func() error {
					// Should have 2 target groups: 1 for backend rule + 1 for mixed rule
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyTargetGroupCount(ctx, 2)
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("verifying target group health", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyTargetGroupHealthy(ctx)
				}, 2*time.Minute, 10*time.Second).Should(Succeed())
			})

			By("cleaning up resources", func() {
				err := stack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("should handle state transitions from redirect-only to backend rules", func() {
			// Start with redirect-only rule
			initialRules := []gwv1.HTTPRouteRule{
				{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
								Value: awssdk.String("/transition"),
							},
						},
					},
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Scheme: awssdk.String("https"),
							},
						},
					},
					// No BackendRefs initially
				},
			}

			httpr := buildHTTPRoute([]string{}, initialRules, nil)

			By("deploying initial redirect-only HTTPRoute", func() {
				stack = newALBResourceStack(
					nil,
					nil,
					buildGatewayClass(),
					buildGateway(gwListeners),
					lbcSpec,
					nil,
					nil,
					[]*gwv1.HTTPRoute{httpr},
					nil,
					nil,
					"transition-test",
					false,
				)

				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, nil, nil, nil, false)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying initial state - no target groups", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyTargetGroupCount(ctx, 0)
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("transitioning to backend rule", func() {
				// Create deployment and service
				deployment := buildDeployment(defaultName, defaultNumReplicas, appContainerPort)
				service := buildService(defaultName, appContainerPort, corev1.ServiceTypeClusterIP)

				// Update HTTPRoute to have backend instead of redirect
				updatedRules := []gwv1.HTTPRouteRule{
					{
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
									Value: awssdk.String("/transition"),
								},
							},
						},
						BackendRefs: DefaultHttpRouteRuleBackendRefs,
						// Remove redirect filters
					},
				}

				// Deploy updated resources
				updatedHttpr := buildHTTPRoute([]string{}, updatedRules, nil)
				
				// Update the stack with backend resources
				stack.Deployments = []*appsv1.Deployment{deployment}
				stack.Services = []*corev1.Service{service}
				stack.TargetGroupConfigurations = []*elbv2gw.TargetGroupConfiguration{tgSpec}
				stack.HTTPRoutes = []*gwv1.HTTPRoute{updatedHttpr}

				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{updatedHttpr}, lbcSpec, tgSpec, nil, nil, true)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying transition - target group created", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyTargetGroupCount(ctx, 1) // Should now have 1 target group
				}, 2*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("verifying target group health after transition", func() {
				Eventually(func() error {
					return verifier.NewALBVerifier(stack.LoadBalancer, tf.LBC.ELBv2Client, tf.LBC.EC2Client, tf.Logger).
						VerifyTargetGroupHealthy(ctx)
				}, 2*time.Minute, 10*time.Second).Should(Succeed())
			})

			By("cleaning up resources", func() {
				err := stack.Cleanup(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Error Scenarios", func() {
		It("should handle invalid redirect configurations gracefully", func() {
			// Create HTTPRoute with invalid redirect configuration
			httpRouteRules := []gwv1.HTTPRouteRule{
				{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  &[]gwv1.PathMatchType{gwv1.PathMatchExact}[0],
								Value: awssdk.String("/invalid-redirect"),
							},
						},
					},
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								StatusCode: awssdk.Int(200), // Invalid status code for redirect
							},
						},
					},
				},
			}

			httpr := buildHTTPRoute([]string{}, httpRouteRules, nil)

			By("deploying HTTPRoute with invalid redirect", func() {
				stack = newALBResourceStack(
					nil,
					nil,
					buildGatewayClass(),
					buildGateway(gwListeners),
					lbcSpec,
					nil,
					nil,
					[]*gwv1.HTTPRoute{httpr},
					nil,
					nil,
					"invalid-redirect-test",
					false,
				)

				err := stack.DeployHTTP(ctx, nil, tf, gwListeners, []*gwv1.HTTPRoute{httpr}, lbcSpec, nil, nil, nil, false)
				// Should handle the error gracefully
				if err != nil {
					// Verify the error is related to validation
					Expect(err.Error()).To(ContainSubstring("status code"))
				}
			})

			By("verifying HTTPRoute status reflects the error", func() {
				// Check that the HTTPRoute status is updated with error information
				Eventually(func() error {
					var route gwv1.HTTPRoute
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: httpr.Namespace,
						Name:      httpr.Name,
					}, &route)
					if err != nil {
						return err
					}

					// Check if status indicates an error
					for _, parent := range route.Status.Parents {
						for _, condition := range parent.Conditions {
							if condition.Type == string(gwv1.RouteConditionAccepted) && condition.Status == metav1.ConditionFalse {
								return nil // Found expected error condition
							}
						}
					}
					return fmt.Errorf("expected error condition not found in HTTPRoute status")
				}, 1*time.Minute, 5*time.Second).Should(Succeed())
			})

			By("cleaning up resources", func() {
				if stack != nil {
					err := stack.Cleanup(ctx, tf)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})
	})
})