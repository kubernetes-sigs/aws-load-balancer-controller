package gateway

import (
	"context"
	"fmt"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("test k8s service reconciled by the aws load balancer controller", func() {
	var (
		ctx     context.Context
		stack   NLBInstanceTestStack
		dnsName string
		lbARN   string
	)
	BeforeEach(func() {
		ctx = context.Background()
		stack = NLBInstanceTestStack{}
	})
	AfterEach(func() {
		err := stack.Cleanup(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
	})
	Context("with NLB instance target configuration", func() {
		BeforeEach(func() {})
		It("should provision internet-facing load balancer resources", func() {
			interf := elbv2gw.LoadBalancerSchemeInternetFacing
			lbcSpec := elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: &interf,
			}
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, lbcSpec)
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

			By("verifying AWS loadbalancer resources", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					TargetType:   "instance",
					Listeners:    stack.resourceStack.getListenersPortMap(),
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					NumTargets:   len(nodeList),
					TargetGroupHC: &verifier.TargetGroupHC{
						Protocol:           "TCP",
						Port:               "traffic-port",
						Interval:           10,
						Timeout:            10,
						HealthyThreshold:   3,
						UnhealthyThreshold: 3,
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			By("waiting for target group targets to be healthy", func() {
				nodeList, err := stack.GetWorkerNodes(ctx, tf)
				Expect(err).ToNot(HaveOccurred())
				err = verifier.WaitUntilTargetsAreHealthy(ctx, tf, lbARN, len(nodeList))
				Expect(err).NotTo(HaveOccurred())
			})
			By("waiting until DNS name is available", func() {
				err := utils.WaitUntilDNSNameAvailable(ctx, dnsName)
				Expect(err).NotTo(HaveOccurred())
			})
			By("sending http request to the lb", func() {
				url := fmt.Sprintf("http://%v/any-path", dnsName)
				err := tf.HTTPVerifier.VerifyURL(url, http.ResponseCodeMatches(200))
				Expect(err).NotTo(HaveOccurred())
			})

			By("enabling cross zone load balancing", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
				})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					return verifier.VerifyLoadBalancerAttributes(ctx, tf, lbARN, map[string]string{
						"load_balancing.cross_zone.enabled": "true",
					}) == nil
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})

			By("specifying load balancer tags", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "instance-mode=true, key1=value1",
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					return verifier.VerifyLoadBalancerResourceTags(ctx, tf, lbARN, map[string]string{
						"instance-mode":            "true",
						"key1":                     "value1",
						"elbv2.k8s.aws/cluster":    tf.Options.ClusterName,
						"service.k8s.aws/stack":    stack.resourceStack.GetStackName(),
						"service.k8s.aws/resource": "*",
					}, nil)
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("modifying load balancer tags", func() {
				err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "instance-mode=true",
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					return verifier.VerifyLoadBalancerResourceTags(ctx, tf, lbARN, map[string]string{
						"instance-mode":            "true",
						"elbv2.k8s.aws/cluster":    tf.Options.ClusterName,
						"service.k8s.aws/stack":    stack.resourceStack.GetStackName(),
						"service.k8s.aws/resource": "*",
					}, map[string]string{
						"key1": "value1",
					})
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())

			})
			By("modifying external traffic policy", func() {
				err := stack.UpdateServiceTrafficPolicy(ctx, tf, corev1.ServiceExternalTrafficPolicyTypeLocal)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					return verifier.GetTargetGroupHealthCheckProtocol(ctx, tf, lbARN) == "HTTP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
				err = verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					TargetType:   "instance",
					Listeners:    stack.resourceStack.getListenersPortMap(),
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					TargetGroupHC: &verifier.TargetGroupHC{
						Protocol:           "HTTP",
						Port:               stack.resourceStack.getHealthCheckNodePort(),
						Path:               "/healthz",
						Interval:           10,
						Timeout:            6,
						HealthyThreshold:   2,
						UnhealthyThreshold: 2,
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			// remove this once listener attributes are available in isolated region
			if !strings.Contains(tf.Options.AWSRegion, "-iso-") {
				By("modifying listener attributes", func() {
					err := stack.UpdateServiceAnnotations(ctx, tf, map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-listener-attributes.TCP-80": "tcp.idle_timeout.seconds=400",
					})
					Expect(err).NotTo(HaveOccurred())

					lsARN := verifier.GetLoadBalancerListenerARN(ctx, tf, lbARN, "80")

					Eventually(func() bool {
						return verifier.VerifyListenerAttributes(ctx, tf, lsARN, map[string]string{
							"tcp.idle_timeout.seconds": "400",
						}) == nil
					}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
				})
			}
		})
	})
})
