package service

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo"
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
		It("should provision internet-facing load balancer resources", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, nil)
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
				nodeList := &corev1.NodeList{}
				err := tf.K8sClient.List(ctx, nodeList)
				Expect(err).ToNot(HaveOccurred())
				err = verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					TargetType:   "instance",
					Listeners:    stack.resourceStack.getListenersPortMap(),
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					NumTargets:   len(nodeList.Items),
					TargetGroupHC: &TargetGroupHC{
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
				nodeList := &corev1.NodeList{}
				err := tf.K8sClient.List(ctx, nodeList)
				Expect(err).ToNot(HaveOccurred())
				err = waitUntilTargetsAreHealthy(ctx, tf, lbARN, len(nodeList.Items))
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
				err := stack.UpdateServiceAnnotation(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
				})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					return verifyLoadBalancerAttributes(ctx, tf, lbARN, map[string]string{
						"load_balancing.cross_zone.enabled": "true",
					}) == nil
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})

			By("specifying load balancer tags", func() {
				err := stack.UpdateServiceAnnotation(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "instance-mode=true, key1=value1",
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					return verifyLoadBalancerTags(ctx, tf, lbARN, map[string]string{
						"instance-mode": "true",
						"key1":          "value1",
					})
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
			By("modifying external traffic policy", func() {
				err := stack.UpdateServiceTrafficPolicy(ctx, tf, corev1.ServiceExternalTrafficPolicyTypeLocal)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					return getTargetGroupHealthCheckProtocol(ctx, tf, lbARN) == "HTTP"
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
				err = verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internet-facing",
					TargetType:   "instance",
					Listeners:    stack.resourceStack.getListenersPortMap(),
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					TargetGroupHC: &TargetGroupHC{
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
		})
		It("should provision internal load-balancer resources", func() {
			By("deploying stack", func() {
				err := stack.Deploy(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-internal": "true",
				})
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
				nodeList := &corev1.NodeList{}
				err := tf.K8sClient.List(ctx, nodeList)
				Expect(err).ToNot(HaveOccurred())
				err = verifyAWSLoadBalancerResources(ctx, tf, lbARN, LoadBalancerExpectation{
					Type:         "network",
					Scheme:       "internal",
					TargetType:   "instance",
					Listeners:    stack.resourceStack.getListenersPortMap(),
					TargetGroups: stack.resourceStack.getTargetGroupNodePortMap(),
					NumTargets:   len(nodeList.Items),
					TargetGroupHC: &TargetGroupHC{
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
			By("specifying target group attributes annotation", func() {
				err := stack.UpdateServiceAnnotation(ctx, tf, map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "preserve_client_ip.enabled=false, proxy_protocol_v2.enabled=true, deregistration_delay.timeout_seconds=120",
				})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					return verifyTargetGroupAttributes(ctx, tf, lbARN, map[string]string{
						"preserve_client_ip.enabled":           "false",
						"proxy_protocol_v2.enabled":            "true",
						"deregistration_delay.timeout_seconds": "120",
					})
				}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
			})
		})
	})
})
