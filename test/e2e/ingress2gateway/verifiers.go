package ingress2gateway

import (
	"context"
	"fmt"
	"slices"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	. "github.com/onsi/gomega"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	frameworkhttp "sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// compareALBConfigurations fetches listeners once for both ALBs and compares listeners, rules, target groups.
func compareALBConfigurations(ctx context.Context, tf *framework.Framework, ingressLBARN, gatewayLBARN string) {
	listeners1, err := tf.LBManager.GetLoadBalancerListeners(ctx, ingressLBARN)
	Expect(err).NotTo(HaveOccurred())
	listeners2, err := tf.LBManager.GetLoadBalancerListeners(ctx, gatewayLBARN)
	Expect(err).NotTo(HaveOccurred())

	Expect(len(listeners2)).To(Equal(len(listeners1)), "listener count mismatch")
	Expect(listenerPortSet(listeners2)).To(Equal(listenerPortSet(listeners1)), "listener ports mismatch")

	listenersByPort1 := listenerByPort(listeners1)
	listenersByPort2 := listenerByPort(listeners2)

	for port, l1 := range listenersByPort1 {
		l2, ok := listenersByPort2[port]
		Expect(ok).To(BeTrue(), "gateway missing listener on port %s", port)

		rules1, err := tf.LBManager.GetLoadBalancerListenerRules(ctx, awssdk.ToString(l1.ListenerArn))
		Expect(err).NotTo(HaveOccurred())
		rules2, err := tf.LBManager.GetLoadBalancerListenerRules(ctx, awssdk.ToString(l2.ListenerArn))
		Expect(err).NotTo(HaveOccurred())

		Expect(len(rules2)).To(Equal(len(rules1)), "listener rule count mismatch for port %s", port)

		nonDefault1 := filterNonDefaultRules(rules1)
		nonDefault2 := filterNonDefaultRules(rules2)
		for j := range nonDefault1 {
			matchRuleConditions(nonDefault1[j], nonDefault2[j], j)
		}

		default1 := findDefaultRule(rules1)
		default2 := findDefaultRule(rules2)
		Expect(default1).NotTo(BeNil(), "ingress listener on port %s missing default rule", port)
		Expect(default2).NotTo(BeNil(), "gateway listener on port %s missing default rule", port)
		matchDefaultRuleActions(default1, default2, port)
	}

	tgs1, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, ingressLBARN)
	Expect(err).NotTo(HaveOccurred())
	tgs2, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, gatewayLBARN)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(tgs2)).To(Equal(len(tgs1)), "target group count mismatch")
	Expect(targetGroupTypeSet(tgs2)).To(Equal(targetGroupTypeSet(tgs1)), "target group types mismatch")
}

// compareTags verifies tags on LB, listener rules, and target groups.
// Gateway resources should have all user tags from ingress plus the migrated-from tag.
func compareTags(ctx context.Context, tf *framework.Framework, ingressLBARN, gatewayLBARN, migrationTagLBValue string, expectedUserTags map[string]string) {
	ingressLBTags := getTagMap(ctx, tf, ingressLBARN)
	gatewayLBTags := getTagMap(ctx, tf, gatewayLBARN)
	Expect(len(gatewayLBTags)).To(Equal(len(ingressLBTags)+1), "gateway LB should have exactly 1 more tag than ingress LB (migrated-from)")
	for k, v := range expectedUserTags {
		Expect(ingressLBTags).To(HaveKeyWithValue(k, v), "user tag %s=%s missing on ingress LB", k, v)
		Expect(gatewayLBTags).To(HaveKeyWithValue(k, v), "user tag %s=%s missing on gateway LB", k, v)
	}
	Expect(gatewayLBTags).To(HaveKeyWithValue(migrationTagKey, migrationTagLBValue),
		"migrated-from tag missing on gateway LB")
	Expect(ingressLBTags).NotTo(HaveKey(migrationTagKey),
		"migrated-from tag should not exist on ingress LB")

	ingressListeners, err := tf.LBManager.GetLoadBalancerListeners(ctx, ingressLBARN)
	Expect(err).NotTo(HaveOccurred())
	gatewayListeners, err := tf.LBManager.GetLoadBalancerListeners(ctx, gatewayLBARN)
	Expect(err).NotTo(HaveOccurred())

	listenersByPort1 := listenerByPort(ingressListeners)
	listenersByPort2 := listenerByPort(gatewayListeners)

	for port, l1 := range listenersByPort1 {
		l2 := listenersByPort2[port]
		ingressRules := getNonDefaultRules(ctx, tf, awssdk.ToString(l1.ListenerArn))
		gatewayRules := getNonDefaultRules(ctx, tf, awssdk.ToString(l2.ListenerArn))
		for j := range ingressRules {
			ingressRuleTags := getTagMap(ctx, tf, awssdk.ToString(ingressRules[j].RuleArn))
			gatewayRuleTags := getTagMap(ctx, tf, awssdk.ToString(gatewayRules[j].RuleArn))
			Expect(len(gatewayRuleTags)).To(Equal(len(ingressRuleTags)+1), "gateway rule %d should have exactly 1 more tag than ingress rule (migrated-from)", j)
			for k, v := range expectedUserTags {
				Expect(ingressRuleTags).To(HaveKeyWithValue(k, v), "user tag %s=%s missing on ingress rule %d", k, v, j)
				Expect(gatewayRuleTags).To(HaveKeyWithValue(k, v), "user tag %s=%s missing on gateway rule %d", k, v, j)
			}
			Expect(gatewayRuleTags).To(HaveKey(migrationTagKey), "migrated-from tag missing on gateway rule %d", j)
			Expect(ingressRuleTags).NotTo(HaveKey(migrationTagKey), "migrated-from tag should not exist on ingress rule %d", j)
		}
	}

	ingressTGs, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, ingressLBARN)
	Expect(err).NotTo(HaveOccurred())
	gatewayTGs, err := tf.TGManager.GetTargetGroupsForLoadBalancer(ctx, gatewayLBARN)
	Expect(err).NotTo(HaveOccurred())

	ingressTGsByHC := targetGroupByHealthCheck(ingressTGs)
	gatewayTGsByHC := targetGroupByHealthCheck(gatewayTGs)

	for hc, ingTG := range ingressTGsByHC {
		gwTG, ok := gatewayTGsByHC[hc]
		Expect(ok).To(BeTrue(), "gateway missing TG with health check %s", hc)

		ingressTGTags := getTagMap(ctx, tf, awssdk.ToString(ingTG.TargetGroupArn))
		gatewayTGTags := getTagMap(ctx, tf, awssdk.ToString(gwTG.TargetGroupArn))
		Expect(len(gatewayTGTags)).To(Equal(len(ingressTGTags)+1), "gateway TG (hc=%s) should have exactly 1 more tag than ingress TG (migrated-from)", hc)
		for k, v := range expectedUserTags {
			Expect(ingressTGTags).To(HaveKeyWithValue(k, v), "user tag %s=%s missing on ingress TG (hc=%s)", k, v, hc)
			Expect(gatewayTGTags).To(HaveKeyWithValue(k, v), "user tag %s=%s missing on gateway TG (hc=%s)", k, v, hc)
		}
		Expect(ingressTGTags).NotTo(HaveKey(migrationTagKey), "migrated-from tag should not exist on ingress TG")
		Expect(gatewayTGTags).To(HaveKey(migrationTagKey), "migrated-from tag missing on gateway TG (hc=%s)", hc)
	}
}

// matchRuleConditions compares two rules by checking that they have the same number of conditions
// and that for each condition field in rule1, rule2 has a matching condition with the same values.
func matchRuleConditions(rule1, rule2 elbv2types.Rule, ruleIndex int) {
	Expect(len(rule2.Conditions)).To(Equal(len(rule1.Conditions)), "rule %d condition count mismatch", ruleIndex)

	for _, c1 := range rule1.Conditions {
		key := conditionKey(c1)
		values1 := conditionValues(c1)
		found := false
		for _, c2 := range rule2.Conditions {
			if conditionKey(c2) == key {
				values2 := conditionValues(c2)
				sorted1 := slices.Sorted(slices.Values(values1))
				sorted2 := slices.Sorted(slices.Values(values2))
				Expect(sorted2).To(Equal(sorted1), "rule %d condition %s values mismatch", ruleIndex, key)
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "rule %d condition %s not found in gateway rule", ruleIndex, key)
	}
}

func conditionKey(c elbv2types.RuleCondition) string {
	field := awssdk.ToString(c.Field)
	if c.HttpHeaderConfig != nil {
		return fmt.Sprintf("%s/%s", field, awssdk.ToString(c.HttpHeaderConfig.HttpHeaderName))
	}
	return field
}

func conditionValues(c elbv2types.RuleCondition) []string {
	if c.HostHeaderConfig != nil {
		return c.HostHeaderConfig.Values
	}
	if c.PathPatternConfig != nil {
		return c.PathPatternConfig.Values
	}
	if c.HttpHeaderConfig != nil {
		return c.HttpHeaderConfig.Values
	}
	if c.HttpRequestMethodConfig != nil {
		return c.HttpRequestMethodConfig.Values
	}
	if c.SourceIpConfig != nil {
		return c.SourceIpConfig.Values
	}
	if c.QueryStringConfig != nil {
		var values []string
		for _, kv := range c.QueryStringConfig.Values {
			values = append(values, fmt.Sprintf("%s=%s", awssdk.ToString(kv.Key), awssdk.ToString(kv.Value)))
		}
		return values
	}
	return nil
}

func getTagMap(ctx context.Context, tf *framework.Framework, resourceARN string) map[string]string {
	tags, err := tf.LBManager.GetLoadBalancerResourceTags(ctx, resourceARN)
	Expect(err).NotTo(HaveOccurred())
	tagMap := make(map[string]string, len(tags))
	for _, t := range tags {
		tagMap[awssdk.ToString(t.Key)] = awssdk.ToString(t.Value)
	}
	return tagMap
}

func getNonDefaultRules(ctx context.Context, tf *framework.Framework, listenerARN string) []elbv2types.Rule {
	rules, err := tf.LBManager.GetLoadBalancerListenerRules(ctx, listenerARN)
	Expect(err).NotTo(HaveOccurred())
	return filterNonDefaultRules(rules)
}

func listenerPortSet(listeners []elbv2types.Listener) map[string]string {
	result := make(map[string]string)
	for _, l := range listeners {
		port := fmt.Sprintf("%d", awssdk.ToInt32(l.Port))
		result[port] = string(l.Protocol)
	}
	return result
}

func listenerByPort(listeners []elbv2types.Listener) map[string]elbv2types.Listener {
	result := make(map[string]elbv2types.Listener, len(listeners))
	for _, l := range listeners {
		port := fmt.Sprintf("%d", awssdk.ToInt32(l.Port))
		result[port] = l
	}
	return result
}

func targetGroupTypeSet(tgs []elbv2types.TargetGroup) map[string]int {
	result := make(map[string]int)
	for _, tg := range tgs {
		result[string(tg.TargetType)]++
	}
	return result
}

func targetGroupByHealthCheck(tgs []elbv2types.TargetGroup) map[string]elbv2types.TargetGroup {
	result := make(map[string]elbv2types.TargetGroup, len(tgs))
	for _, tg := range tgs {
		key := fmt.Sprintf("%s:%s", awssdk.ToString(tg.HealthCheckPath), awssdk.ToString(tg.HealthCheckPort))
		result[key] = tg
	}
	return result
}

func filterNonDefaultRules(rules []elbv2types.Rule) []elbv2types.Rule {
	var result []elbv2types.Rule
	for _, r := range rules {
		if !awssdk.ToBool(r.IsDefault) {
			result = append(result, r)
		}
	}
	return result
}

func findDefaultRule(rules []elbv2types.Rule) *elbv2types.Rule {
	for i := range rules {
		if awssdk.ToBool(rules[i].IsDefault) {
			return &rules[i]
		}
	}
	return nil
}

func matchDefaultRuleActions(rule1, rule2 *elbv2types.Rule, port string) {
	Expect(len(rule2.Actions)).To(Equal(len(rule1.Actions)), "default rule action count mismatch for port %s", port)
	for i := range rule1.Actions {
		Expect(string(rule2.Actions[i].Type)).To(Equal(string(rule1.Actions[i].Type)),
			"default rule action %d type mismatch for port %s", i, port)
	}
}

type trafficCase struct {
	path     string
	host     string
	expected string
}

// verifyTraffic sends HTTP requests to the LB and verifies expected responses.
func verifyTraffic(tf *framework.Framework, lbDNS string, cases []trafficCase) {
	for _, tc := range cases {
		url := fmt.Sprintf("http://%s%s", lbDNS, tc.path)
		err := tf.HTTPVerifier.VerifyURLWithOptions(url, frameworkhttp.URLOptions{
			HostHeader: tc.host,
		}, frameworkhttp.ResponseCodeMatches(200), frameworkhttp.ResponseBodyMatches([]byte(tc.expected)))
		Expect(err).NotTo(HaveOccurred(), "traffic verification failed for %s %s", tc.host, tc.path)
	}
}

type expectedResourceCounts struct {
	gateways                   int
	httpRoutes                 int
	loadBalancerConfigurations int
	targetGroupConfigurations  int
	listenerRuleConfigurations int
}

// verifyResourceCounts checks that the migration produced exactly the expected number of each resource kind.
func verifyResourceCounts(ctx context.Context, tf *framework.Framework, namespace string, expected expectedResourceCounts) {
	gwList := &gwv1.GatewayList{}
	Expect(tf.K8sClient.List(ctx, gwList, client.InNamespace(namespace))).To(Succeed())
	Expect(gwList.Items).To(HaveLen(expected.gateways), "Gateway count mismatch")

	routeList := &gwv1.HTTPRouteList{}
	Expect(tf.K8sClient.List(ctx, routeList, client.InNamespace(namespace))).To(Succeed())
	Expect(routeList.Items).To(HaveLen(expected.httpRoutes), "HTTPRoute count mismatch")

	lbcList := &elbv2gw.LoadBalancerConfigurationList{}
	Expect(tf.K8sClient.List(ctx, lbcList, client.InNamespace(namespace))).To(Succeed())
	Expect(lbcList.Items).To(HaveLen(expected.loadBalancerConfigurations), "LoadBalancerConfiguration count mismatch")

	tgcList := &elbv2gw.TargetGroupConfigurationList{}
	Expect(tf.K8sClient.List(ctx, tgcList, client.InNamespace(namespace))).To(Succeed())
	Expect(tgcList.Items).To(HaveLen(expected.targetGroupConfigurations), "TargetGroupConfiguration count mismatch")

	lrcList := &elbv2gw.ListenerRuleConfigurationList{}
	Expect(tf.K8sClient.List(ctx, lrcList, client.InNamespace(namespace))).To(Succeed())
	Expect(lrcList.Items).To(HaveLen(expected.listenerRuleConfigurations), "ListenerRuleConfiguration count mismatch")
}
