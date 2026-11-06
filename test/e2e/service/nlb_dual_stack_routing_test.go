package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	httputils "sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/utils"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/verifier"
)

// hasLocalIPv6 returns true when the test runner has at least one non-loopback
// IPv6 address, indicating it can make outbound IPv6 connections.
func hasLocalIPv6() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() == nil && ip.IsGlobalUnicast() {
				return true
			}
		}
	}
	return false
}

// ipv6HTTPClient returns an http.Client that forces IPv6 by resolving only
// AAAA records and dialing over IPv6.
func ipv6HTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, err
				}
				for _, a := range addrs {
					if a.IP.To4() == nil {
						return dialer.DialContext(ctx, "tcp6", net.JoinHostPort(a.IP.String(), port))
					}
				}
				return nil, fmt.Errorf("no AAAA record found for %s", host)
			},
		},
	}
}

// TestDualStackNLBRoutingStack holds a single deployment + dual-stack LoadBalancer service.
type TestDualStackNLBRoutingStack struct {
	resourceStack *ResourceStack
}

func (s *TestDualStackNLBRoutingStack) Deploy(ctx context.Context, f *framework.Framework) error {
	dp := (&NLBInstanceTestStack{}).buildDeploymentSpec(f.Options.TestImageRegistry)
	svc := s.buildServiceSpec()
	s.resourceStack = NewResourceStack(dp, svc, nil, nil, "nlb-ds-routing", map[string]string{})
	return s.resourceStack.Deploy(ctx, f)
}

func (s *TestDualStackNLBRoutingStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if s.resourceStack == nil {
		return nil
	}
	return s.resourceStack.Cleanup(ctx, f)
}

func (s *TestDualStackNLBRoutingStack) GetLoadBalancerIngressHostName() string {
	return s.resourceStack.GetLoadBalancerIngressHostname()
}

func (s *TestDualStackNLBRoutingStack) buildServiceSpec() *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":     "multi-port",
		"app.kubernetes.io/instance": defaultName,
	}
	policy := corev1.IPFamilyPolicyRequireDualStack
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nlb-ds-routing",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type":               "external",
				"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":    "instance",
				"service.beta.kubernetes.io/aws-load-balancer-scheme":             "internet-facing",
				"service.beta.kubernetes.io/aws-load-balancer-ip-address-type":    "dualstack",
				"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Type:           corev1.ServiceTypeLoadBalancer,
			Selector:       labels,
			IPFamilyPolicy: &policy,
			IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

var _ = Describe("NLB dual-stack routing", func() {
	var (
		ctx   context.Context
		stack TestDualStackNLBRoutingStack
		lbARN string
		lsARN string
	)

	BeforeEach(func() {
		// Dual-stack pod networking (both IPv4 and IPv6 in spec.ipFamilies) is not
		// supported by the Amazon VPC CNI, so this test cannot run on EKS.
		// Re-enable once EKS supports dual-stack routing by passing --ip-family=DualStack.
		Skip("dual-stack routing requires a dual-stack cluster not yet supported by EKS; re-enable when EKS supports it")

		ctx = context.Background()
		stack = TestDualStackNLBRoutingStack{}
	})

	AfterEach(func() {
		Expect(stack.Cleanup(ctx, tf)).To(Succeed())
	})

	It("provisions an IPv4 and IPv6 target group with a priority-1 source-IP listener rule", func() {
		By("deploying the dual-stack service stack", func() {
			Expect(stack.Deploy(ctx, tf)).To(Succeed())
		})

		var dnsName string
		By("checking service status for LB DNS name", func() {
			dnsName = stack.GetLoadBalancerIngressHostName()
			Expect(dnsName).ToNot(BeEmpty())
			GinkgoWriter.Printf("LB DNS name: %s\n", dnsName)
		})

		By("resolving LB ARN from DNS name", func() {
			var err error
			lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
			Expect(err).NotTo(HaveOccurred())
			Expect(lbARN).ToNot(BeEmpty())
			GinkgoWriter.Printf("LB ARN: %s\n", lbARN)
		})

		By("verifying the NLB is dual-stack", func() {
			lb, err := tf.LBManager.GetLoadBalancerFromARN(ctx, lbARN)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(lb.IpAddressType)).To(Equal("dualstack"))
			GinkgoWriter.Printf("LB IP address type: %s\n", lb.IpAddressType)
		})

		By("resolving the listener ARN for port 80", func() {
			listeners, err := tf.LBManager.GetLoadBalancerListeners(ctx, lbARN)
			Expect(err).NotTo(HaveOccurred())
			Expect(listeners).To(HaveLen(1))
			lsARN = awssdk.ToString(listeners[0].ListenerArn)
			GinkgoWriter.Printf("listener ARN: %s\n", lsARN)
		})

		By("verifying a priority-1 listener rule with source-ip=ipv4 condition exists", func() {
			rules, err := tf.LBManager.GetLoadBalancerListenerRules(ctx, lsARN)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("found %d listener rules\n", len(rules))

			var ipv4Rule *elbv2types.Rule
			for i, r := range rules {
				if awssdk.ToString(r.Priority) == "1" {
					ipv4Rule = &rules[i]
					break
				}
			}
			Expect(ipv4Rule).NotTo(BeNil(), "expected a priority-1 listener rule")
			Expect(ipv4Rule.Conditions).To(HaveLen(1))

			cond := ipv4Rule.Conditions[0]
			Expect(awssdk.ToString(cond.Field)).To(Equal("source-ip"))
			Expect(cond.SourceIpConfig).NotTo(BeNil())
			Expect(string(cond.SourceIpConfig.IpAddressType)).To(Equal("ipv4"),
				"priority-1 rule must match IPv4 source traffic")
			GinkgoWriter.Printf("priority-1 rule: field=%s ipAddressType=%s\n",
				awssdk.ToString(cond.Field), cond.SourceIpConfig.IpAddressType)
		})

		By("verifying two target groups exist — one IPv4, one IPv6", func() {
			tgs, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
			Expect(err).NotTo(HaveOccurred())
			Expect(tgs).To(HaveLen(2), "expected exactly two target groups (one IPv4, one IPv6)")

			ipFamilies := make(map[string]bool)
			for _, tg := range tgs {
				ipFamilies[string(tg.IpAddressType)] = true
				GinkgoWriter.Printf("target group: name=%s ipAddressType=%s arn=%s\n",
					awssdk.ToString(tg.TargetGroupName), tg.IpAddressType, awssdk.ToString(tg.TargetGroupArn))
			}
			Expect(ipFamilies["ipv4"]).To(BeTrue(), "expected an IPv4 target group")
			Expect(ipFamilies["ipv6"]).To(BeTrue(), "expected an IPv6 target group")
		})

		nodeList, err := (&NLBInstanceTestStack{resourceStack: stack.resourceStack}).GetWorkerNodes(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("worker nodes: %d\n", len(nodeList))

		By("verifying AWS load balancer resources", func() {
			nodePort := stack.resourceStack.createdSVC.Spec.Ports[0].NodePort
			GinkgoWriter.Printf("service node port: %d\n", nodePort)
			expectedTGs := []verifier.ExpectedTargetGroup{
				{Protocol: "TCP", Port: nodePort, NumTargets: len(nodeList), TargetType: "instance"},
				{Protocol: "TCP", Port: nodePort, NumTargets: len(nodeList), TargetType: "instance"},
			}
			Expect(verifier.VerifyAWSLoadBalancerResources(ctx, tf, lbARN, verifier.LoadBalancerExpectation{
				Type:         "network",
				Scheme:       "internet-facing",
				Listeners:    stack.resourceStack.getListenersPortMap(),
				TargetGroups: expectedTGs,
			})).To(Succeed())
		})

		By("waiting for targets to be healthy in both target groups", func() {
			tgs, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
			Expect(err).NotTo(HaveOccurred())
			for _, tg := range tgs {
				tgARN := awssdk.ToString(tg.TargetGroupArn)
				Eventually(func() bool {
					ok, err := tf.TGManager.CheckTargetGroupHealthy(ctx, tgARN, len(nodeList))
					return err == nil && ok
				}, utils.PollTimeoutLong, utils.PollIntervalLong).Should(BeTrue(),
					"targets in TG %s should become healthy", tgARN)
				GinkgoWriter.Printf("targets healthy in TG: %s (ipAddressType=%s)\n",
					tgARN, tg.IpAddressType)
			}
		})

		By("waiting until DNS name is available", func() {
			Expect(utils.WaitUntilDNSNameAvailable(ctx, dnsName)).To(Succeed())
			GinkgoWriter.Printf("DNS name available: %s\n", dnsName)
		})

		By("sending an HTTP request through the NLB (exercises the IPv4 listener rule path)", func() {
			url := fmt.Sprintf("http://%v/any-path", dnsName)
			Expect(tf.HTTPVerifier.VerifyURL(url, httputils.ResponseCodeMatches(200))).To(Succeed())
			GinkgoWriter.Printf("IPv4 HTTP request succeeded: %s\n", url)
		})

		By("sending an IPv6 HTTP request through the NLB if the test runner has IPv6 connectivity", func() {
			if !hasLocalIPv6() {
				GinkgoWriter.Println("skipping IPv6 traffic check: test runner has no global-unicast IPv6 address")
				return
			}
			// Confirms the NLB accepts IPv6 connections end-to-end. This does not
			// verify the routing split itself — the structural assertions above
			// (priority-1 rule, two TGs with distinct ipAddressType) cover that.
			url := fmt.Sprintf("http://%v/any-path", dnsName)
			resp, err := ipv6HTTPClient().Get(url)
			Expect(err).NotTo(HaveOccurred(), "IPv6 HTTP request to NLB failed")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200), "expected HTTP 200 from NLB over IPv6")
			GinkgoWriter.Printf("IPv6 HTTP request succeeded: %s (status=%d)\n", url, resp.StatusCode)
		})
	})
})
