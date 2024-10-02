package service

import (
	"context"
	"sort"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

type TargetGroupHC struct {
	Protocol           string
	Path               string
	Port               string
	Interval           int32
	Timeout            int32
	HealthyThreshold   int32
	UnhealthyThreshold int32
}

type LoadBalancerExpectation struct {
	Name          string
	Type          string
	Scheme        string
	TargetType    string
	Listeners     map[string]string // listener port, protocol
	TargetGroups  map[string]string // target group port, protocol
	NumTargets    int
	TargetGroupHC *TargetGroupHC
}

func verifyAWSLoadBalancerResources(ctx context.Context, f *framework.Framework, lbARN string, expected LoadBalancerExpectation) error {
	lb, err := f.LBManager.GetLoadBalancerFromARN(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())
	err = verifyLoadBalancerName(ctx, f, lb, expected.Name)
	Expect(err).NotTo(HaveOccurred())
	err = verifyLoadBalancerType(ctx, f, lb, expected.Type, expected.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = verifyLoadBalancerListeners(ctx, f, lbARN, expected.Listeners)
	Expect(err).NotTo(HaveOccurred())
	err = verifyLoadBalancerTargetGroups(ctx, f, lbARN, expected)
	Expect(err).NotTo(HaveOccurred())
	return nil
}

func verifyLoadBalancerName(_ context.Context, f *framework.Framework, lb *elbv2types.LoadBalancer, lbName string) error {
	if len(lbName) > 0 {
		Expect(awssdk.ToString(lb.LoadBalancerName)).To(Equal(lbName))
	}
	return nil
}

func verifyLoadBalancerType(_ context.Context, f *framework.Framework, lb *elbv2types.LoadBalancer, lbType, lbScheme string) error {
	Expect(string(lb.Type)).To(Equal(lbType))
	Expect(string(lb.Scheme)).To(Equal(lbScheme))
	return nil
}

func verifyLoadBalancerAttributes(ctx context.Context, f *framework.Framework, lbARN string, expectedAttrs map[string]string) error {
	lbAttrs, err := f.LBManager.GetLoadBalancerAttributes(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())
	for _, attr := range lbAttrs {
		if val, ok := expectedAttrs[awssdk.ToString(attr.Key)]; ok && val != awssdk.ToString(attr.Value) {
			return errors.Errorf("Attribute %v, expected %v, actual %v", awssdk.ToString(attr.Key), val, awssdk.ToString(attr.Value))
		}
	}
	return nil
}

func verifyLoadBalancerResourceTags(ctx context.Context, f *framework.Framework, lbARN string, expectedTags map[string]string,
	unexpectedTags map[string]string) bool {
	resARNs := []string{lbARN}
	targetGroups, err := f.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())

	for _, tg := range targetGroups {
		resARNs = append(resARNs, awssdk.ToString(tg.TargetGroupArn))
	}

	listeners, err := f.LBManager.GetLoadBalancerListeners(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())
	for _, ls := range listeners {
		resARNs = append(resARNs, awssdk.ToString(ls.ListenerArn))
		rules, err := f.LBManager.GetLoadBalancerListenerRules(ctx, awssdk.ToString(ls.ListenerArn))
		Expect(err).NotTo(HaveOccurred())
		for _, rule := range rules {
			if awssdk.ToBool(rule.IsDefault) {
				continue
			}
			resARNs = append(resARNs, awssdk.ToString(rule.RuleArn))
		}
	}
	for _, resARN := range resARNs {
		if !matchResourceTags(ctx, f, resARN, expectedTags, unexpectedTags) {
			return false
		}
	}
	return true
}

func matchResourceTags(ctx context.Context, f *framework.Framework, resARN string, expectedTags map[string]string, unexpectedTags map[string]string) bool {
	lbTags, err := f.LBManager.GetLoadBalancerResourceTags(ctx, resARN)
	Expect(err).NotTo(HaveOccurred())
	matchedTags := 0
	for _, tag := range lbTags {
		if val, ok := expectedTags[awssdk.ToString(tag.Key)]; ok && (val == "*" || val == awssdk.ToString(tag.Value)) {
			matchedTags++
		}
	}
	for _, tag := range lbTags {
		if val, ok := unexpectedTags[awssdk.ToString(tag.Key)]; ok && (val == "*" || val == awssdk.ToString(tag.Value)) {
			return false
		}
	}
	return matchedTags == len(expectedTags)
}

func getLoadBalancerListenerProtocol(ctx context.Context, f *framework.Framework, lbARN string, port string) string {
	protocol := ""
	listeners, err := f.LBManager.GetLoadBalancerListeners(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	for _, ls := range listeners {
		if strconv.Itoa(int(awssdk.ToInt32(ls.Port))) == port {
			protocol = string(ls.Protocol)
		}
	}
	return protocol
}

func verifyLoadBalancerListeners(ctx context.Context, f *framework.Framework, lbARN string, listenersMap map[string]string) error {
	listeners, err := f.LBManager.GetLoadBalancerListeners(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(listeners)).To(Equal(len(listenersMap)))

	for _, ls := range listeners {
		portStr := strconv.Itoa(int(awssdk.ToInt32(ls.Port)))
		Expect(listenersMap).Should(HaveKey(portStr))
		Expect(string(ls.Protocol)).To(Equal(listenersMap[portStr]))
	}
	return nil
}

func verifyLoadBalancerListenerCertificates(ctx context.Context, f *framework.Framework, lbARN string, expectedCertARNS []string) error {
	listeners, err := f.LBManager.GetLoadBalancerListeners(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(listeners)).Should(BeNumerically(">", 0))
	listenerCerts, err := f.LBManager.GetLoadBalancerListenerCertificates(ctx, awssdk.ToString(listeners[0].ListenerArn))
	Expect(err).ToNot(HaveOccurred())

	var observedCertArns []string
	var defaultCert string
	for _, cert := range listenerCerts {
		if awssdk.ToBool(cert.IsDefault) {
			defaultCert = awssdk.ToString(cert.CertificateArn)
		}
		observedCertArns = append(observedCertArns, awssdk.ToString(cert.CertificateArn))
	}
	if defaultCert != expectedCertARNS[0] {
		return errors.New("default cert does not match")
	}
	//Expect(defaultCert).To(Equal(expectedCertARNS[0]))
	if len(expectedCertARNS) != len(observedCertArns) {
		return errors.New("cert len mismatch")
	}
	sort.Strings(observedCertArns)
	sort.Strings(expectedCertARNS)
	Expect(expectedCertARNS).To(Equal(observedCertArns))
	return nil
}

func verifyLoadBalancerTargetGroups(ctx context.Context, f *framework.Framework, lbARN string, expected LoadBalancerExpectation) error {
	targetGroups, err := f.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(targetGroups)).To(Equal(len(expected.TargetGroups)))
	for _, tg := range targetGroups {
		Expect(string(tg.TargetType)).To(Equal(expected.TargetType))
		Expect(string(tg.Protocol)).To(Equal(expected.TargetGroups[strconv.Itoa(int(awssdk.ToInt32(tg.Port)))]))
		err = verifyTargetGroupHealthCheckConfig(tg, expected.TargetGroupHC)
		Expect(err).NotTo(HaveOccurred())
		err = verifyTargetGroupNumRegistered(ctx, f, awssdk.ToString(tg.TargetGroupArn), expected.NumTargets)
		Expect(err).NotTo(HaveOccurred())
	}
	return nil
}

func verifyTargetGroupHealthCheckConfig(tg elbv2types.TargetGroup, hc *TargetGroupHC) error {
	if hc != nil {
		Expect(string(tg.HealthCheckProtocol)).To(Equal(hc.Protocol))
		Expect(awssdk.ToString(tg.HealthCheckPath)).To(Equal(hc.Path))
		Expect(awssdk.ToString(tg.HealthCheckPort)).To(Equal(hc.Port))
		Expect(awssdk.ToInt32(tg.HealthCheckIntervalSeconds)).To(Equal(hc.Interval))
		Expect(awssdk.ToInt32(tg.HealthCheckTimeoutSeconds)).To(Equal(hc.Timeout))
		Expect(awssdk.ToInt32(tg.HealthyThresholdCount)).To(Equal(hc.HealthyThreshold))
		Expect(awssdk.ToInt32(tg.UnhealthyThresholdCount)).To(Equal(hc.UnhealthyThreshold))
	}
	return nil
}

func verifyTargetGroupNumRegistered(ctx context.Context, f *framework.Framework, tgARN string, expectedTargets int) error {
	if expectedTargets == 0 {
		return nil
	}
	Eventually(func() bool {
		numTargets, err := f.TGManager.GetCurrentTargetCount(ctx, tgARN)
		Expect(err).ToNot(HaveOccurred())
		return numTargets == expectedTargets
	}, utils.PollTimeoutShort, utils.PollIntervalMedium).Should(BeTrue())
	return nil
}

func waitUntilTargetsAreHealthy(ctx context.Context, f *framework.Framework, lbARN string, expectedTargetCount int) error {
	targetGroups, err := f.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(targetGroups)).To(Not(BeZero()))
	// Check the first target group
	tgARN := awssdk.ToString(targetGroups[0].TargetGroupArn)

	Eventually(func() (bool, error) {
		return f.TGManager.CheckTargetGroupHealthy(ctx, tgARN, expectedTargetCount)
	}, utils.PollTimeoutLong, utils.PollIntervalLong).Should(BeTrue())
	return nil
}

func getTargetGroupHealthCheckProtocol(ctx context.Context, f *framework.Framework, lbARN string) string {
	targetGroups, err := f.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	return string(targetGroups[0].HealthCheckProtocol)
}

func verifyTargetGroupAttributes(ctx context.Context, f *framework.Framework, lbARN string, expectedAttributes map[string]string) bool {
	targetGroups, err := f.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(targetGroups)).To(Not(BeZero()))
	// Check the first target group
	tgARN := awssdk.ToString(targetGroups[0].TargetGroupArn)
	tgAttrs, err := f.TGManager.GetTargetGroupAttributes(ctx, tgARN)
	Expect(err).NotTo(HaveOccurred())
	matchedAttrs := 0
	for _, attr := range tgAttrs {
		if val, ok := expectedAttributes[awssdk.ToString(attr.Key)]; ok && val == awssdk.ToString(attr.Value) {
			matchedAttrs++
		}
	}
	return matchedAttrs == len(expectedAttributes)
}

func verifyListenerAttributes(ctx context.Context, f *framework.Framework, lsARN string, expectedAttrs map[string]string) error {
	lsAttrs, err := f.LBManager.GetListenerAttributes(ctx, lsARN)
	Expect(err).NotTo(HaveOccurred())
	for _, attr := range lsAttrs {
		if val, ok := expectedAttrs[awssdk.ToString(attr.Key)]; ok && val != awssdk.ToString(attr.Value) {
			return errors.Errorf("Attribute %v, expected %v, actual %v", awssdk.ToString(attr.Key), val, awssdk.ToString(attr.Value))
		}
	}
	return nil
}

func getLoadBalancerListenerARN(ctx context.Context, f *framework.Framework, lbARN string, port string) string {
	lsARN := ""
	listeners, err := f.LBManager.GetLoadBalancerListeners(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	for _, ls := range listeners {
		if strconv.Itoa(int(awssdk.ToInt32(ls.Port))) == port {
			lsARN = awssdk.ToString(ls.ListenerArn)
		}
	}
	return lsARN
}
