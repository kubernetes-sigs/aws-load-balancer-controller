package verifier

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/strings/slices"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sort"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
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

type ExpectedTargetGroup struct {
	Protocol      string
	Port          int32
	NumTargets    int
	TargetType    string
	TargetGroupHC *TargetGroupHC
}

type LoadBalancerExpectation struct {
	Name         string
	Type         string
	Scheme       string
	Listeners    map[string]string // listener port, protocol
	TargetGroups []ExpectedTargetGroup
}

// ListenerExpectation contains the expected configuration for an ALB/NLB listener
// to be verified against actual AWS resources
type ListenerExpectation struct {
	ProtocolPort              string
	DefaultCertificateARN     string
	AdditionalCertificateARNs []string
	SSLPolicy                 string
	ALPNPolicy                string
	MutualAuthentication      *MutualAuthenticationExpectation
	Attributes                map[string]string
}

// MutualAuthenticationExpectation contains expected mTLS settings
type MutualAuthenticationExpectation struct {
	Mode                          string
	TrustStoreARN                 string
	IgnoreClientCertificateExpiry bool
	AdvertiseTrustStoreCaNames    string
}

type ListenerRuleExpectation struct {
	Conditions []elbv2types.RuleCondition
	Actions    []elbv2types.Action
	Priority   int32
}

func VerifyAWSLoadBalancerResources(ctx context.Context, f *framework.Framework, lbARN string, expected LoadBalancerExpectation) error {
	lb, err := f.LBManager.GetLoadBalancerFromARN(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())
	err = VerifyLoadBalancerName(ctx, f, lb, expected.Name)
	Expect(err).NotTo(HaveOccurred())
	err = VerifyLoadBalancerType(ctx, f, lb, expected.Type, expected.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = VerifyLoadBalancerListeners(ctx, f, lbARN, expected.Listeners)
	Expect(err).NotTo(HaveOccurred())
	err = VerifyLoadBalancerTargetGroups(ctx, f, lbARN, expected)
	Expect(err).NotTo(HaveOccurred())
	return nil
}

func VerifyLoadBalancerName(_ context.Context, f *framework.Framework, lb *elbv2types.LoadBalancer, lbName string) error {
	if len(lbName) > 0 {
		Expect(awssdk.ToString(lb.LoadBalancerName)).To(Equal(lbName))
	}
	return nil
}

func VerifyLoadBalancerType(_ context.Context, f *framework.Framework, lb *elbv2types.LoadBalancer, lbType, lbScheme string) error {
	Expect(string(lb.Type)).To(Equal(lbType))
	Expect(string(lb.Scheme)).To(Equal(lbScheme))
	return nil
}

func VerifyLoadBalancerAttributes(ctx context.Context, f *framework.Framework, lbARN string, expectedAttrs map[string]string) error {
	lbAttrs, err := f.LBManager.GetLoadBalancerAttributes(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())
	for _, attr := range lbAttrs {
		if val, ok := expectedAttrs[awssdk.ToString(attr.Key)]; ok && val != awssdk.ToString(attr.Value) {
			return errors.Errorf("Attribute %v, expected %v, actual %v", awssdk.ToString(attr.Key), val, awssdk.ToString(attr.Value))
		}
	}
	return nil
}

func VerifyLoadBalancerResourceTags(ctx context.Context, f *framework.Framework, lbARN string, expectedTags map[string]string,
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
		if !MatchResourceTags(ctx, f, resARN, expectedTags, unexpectedTags) {
			return false
		}
	}
	return true
}

func MatchResourceTags(ctx context.Context, f *framework.Framework, resARN string, expectedTags map[string]string, unexpectedTags map[string]string) bool {
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

func GetLoadBalancerListenerProtocol(ctx context.Context, f *framework.Framework, lbARN string, port string) string {
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

func VerifyLoadBalancerListeners(ctx context.Context, f *framework.Framework, lbARN string, listenersMap map[string]string) error {
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

func VerifyLoadBalancerListenerCertificates(ctx context.Context, f *framework.Framework, lbARN string, expectedCertARNS []string) error {
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

func VerifyLoadBalancerTargetGroups(ctx context.Context, f *framework.Framework, lbARN string, expected LoadBalancerExpectation) error {
	targetGroups, err := f.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())

	Expect(len(targetGroups)).To(Equal(len(expected.TargetGroups)))

	validatedTGs := sets.New[string]() // TG ARNs that have already mapped to another expected TG.
	for _, expectedTg := range expected.TargetGroups {

		for _, awsTg := range targetGroups {
			if awssdk.ToInt32(awsTg.Port) == expectedTg.Port && string(awsTg.Protocol) == expectedTg.Protocol && string(awsTg.TargetType) == expectedTg.TargetType && !validatedTGs.Has(*awsTg.TargetGroupArn) {

				var hcErr error
				if expectedTg.TargetGroupHC != nil {
					hcErr = VerifyTargetGroupHealthCheckConfig(awsTg, expectedTg.TargetGroupHC)
					Expect(hcErr).NotTo(HaveOccurred())
				}

				registeredTargetsErr := VerifyTargetGroupNumRegistered(ctx, f, awssdk.ToString(awsTg.TargetGroupArn), expectedTg.NumTargets)
				Expect(registeredTargetsErr).NotTo(HaveOccurred())

				if hcErr == nil && registeredTargetsErr == nil {
					validatedTGs.Insert(*awsTg.TargetGroupArn)
				}
			}
		}
	}

	if len(validatedTGs) != len(expected.TargetGroups) {
		return fmt.Errorf("target group mismatch expected [%+v] got [%+v]\n", expected.TargetGroups, targetGroups)
	}
	return nil
}

func VerifyTargetGroupHealthCheckConfig(tg elbv2types.TargetGroup, hc *TargetGroupHC) error {
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

func VerifyTargetGroupNumRegistered(ctx context.Context, f *framework.Framework, tgARN string, expectedTargets int) error {
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

func WaitUntilTargetsAreHealthy(ctx context.Context, f *framework.Framework, lbARN string, expectedTargetCount int) error {
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

func GetTargetGroupHealthCheckProtocol(ctx context.Context, f *framework.Framework, lbARN string) string {
	targetGroups, err := f.TGManager.GetTargetGroupsForLoadBalancer(ctx, lbARN)
	Expect(err).ToNot(HaveOccurred())
	return string(targetGroups[0].HealthCheckProtocol)
}

func VerifyTargetGroupAttributes(ctx context.Context, f *framework.Framework, lbARN string, expectedAttributes map[string]string) bool {
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

func VerifyListenerAttributes(ctx context.Context, f *framework.Framework, lsARN string, expectedAttrs map[string]string) error {
	lsAttrs, err := f.LBManager.GetListenerAttributes(ctx, lsARN)
	Expect(err).NotTo(HaveOccurred())
	for _, attr := range lsAttrs {
		if val, ok := expectedAttrs[awssdk.ToString(attr.Key)]; ok && val != awssdk.ToString(attr.Value) {
			return errors.Errorf("Attribute %v, expected %v, actual %v", awssdk.ToString(attr.Key), val, awssdk.ToString(attr.Value))
		}
	}
	return nil
}

func GetLoadBalancerListenerARN(ctx context.Context, f *framework.Framework, lbARN string, port string) string {
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

func VerifyLoadBalancerListener(ctx context.Context, f *framework.Framework, lbARN string, port int32, expectedListener *ListenerExpectation) error {
	lsARN := GetLoadBalancerListenerARN(ctx, f, lbARN, strconv.Itoa(int(port)))
	if lsARN == "" && expectedListener == nil {
		return nil
	}
	if lsARN == "" {
		return errors.Errorf("Listener on port %v, expected but none found", port)
	}
	ls, err := f.LBManager.GetLoadBalancerListenerFromARN(ctx, lsARN)
	Expect(err).NotTo(HaveOccurred())

	expectedProtocolPort := strings.Split(expectedListener.ProtocolPort, ":")
	expectedProtocol, expectedPortStr := expectedProtocolPort[0], expectedProtocolPort[1]
	// Verify protocol matches
	if string(ls.Protocol) != expectedProtocol {
		return errors.Errorf("expected listener protocol %s, got %s", expectedProtocol, string(ls.Protocol))
	}
	// Verify port matches
	if strconv.Itoa(int(awssdk.ToInt32(ls.Port))) != expectedPortStr {
		return errors.Errorf("expected listener port %s, got %d", expectedPortStr, awssdk.ToInt32(ls.Port))
	}

	if expectedListener.DefaultCertificateARN != "" {
		// Get certificates for the listener
		certs, err := f.LBManager.GetLoadBalancerListenerCertificates(ctx, lsARN)
		Expect(err).ToNot(HaveOccurred())
		defaultCertFound := false
		for _, cert := range certs {
			if awssdk.ToBool(cert.IsDefault) && awssdk.ToString(cert.CertificateArn) == expectedListener.DefaultCertificateARN {
				defaultCertFound = true
				break
			}
		}
		if !defaultCertFound {
			return errors.Errorf("default certificate %s not found on listener", expectedListener.DefaultCertificateARN)
		}
	}

	err = VerifyListenerMutualAuthentication(ls.MutualAuthentication, expectedListener.MutualAuthentication)
	Expect(err).NotTo(HaveOccurred())

	return nil
}

// TODO: Add "verify mode" verification later after adding a trust-store to the prow test account
func VerifyListenerMutualAuthentication(lsMutualAuth *elbv2types.MutualAuthenticationAttributes, expectedListenerMutualAuthentication *MutualAuthenticationExpectation) error {
	if expectedListenerMutualAuthentication != nil {
		Expect(awssdk.ToString(lsMutualAuth.Mode)).To(Equal(expectedListenerMutualAuthentication.Mode))
	}
	return nil
}

func VerifyLoadBalancerListenerRules(ctx context.Context, f *framework.Framework, lbARN string, port int32, expectedRules []ListenerRuleExpectation) error {
	lsARN := GetLoadBalancerListenerARN(ctx, f, lbARN, strconv.Itoa(int(port)))
	if lsARN == "" && expectedRules == nil {
		return nil
	}
	if lsARN == "" {
		return errors.Errorf("Listener on port %v, expected but none found", port)
	}

	rules, err := f.LBManager.GetLoadBalancerListenerRules(ctx, lsARN)
	if err != nil {
		return err
	}

	var filteredRules []elbv2types.Rule
	// Filter out default rules
	for _, rule := range rules {
		if !awssdk.ToBool(rule.IsDefault) {
			filteredRules = append(filteredRules, rule)
		}
	}

	if len(filteredRules) != len(expectedRules) {
		return errors.Errorf("expected %d listener rules, got %d", len(expectedRules), len(filteredRules))
	}
	// sort actual and expected rules based on priority
	sort.Slice(expectedRules, func(i, j int) bool {
		return expectedRules[i].Priority < expectedRules[j].Priority
	})
	sort.Slice(filteredRules, func(i, j int) bool {
		priorityI, _ := strconv.Atoi(awssdk.ToString(filteredRules[i].Priority))
		priorityJ, _ := strconv.Atoi(awssdk.ToString(filteredRules[j].Priority))
		return priorityI < priorityJ
	})
	// compare priority, actions and conditions
	for i, expectedRule := range expectedRules {
		actualRule := filteredRules[i]
		actualPriority, _ := strconv.Atoi(awssdk.ToString(actualRule.Priority))
		if err := verifyListenerRulePriority(int32(actualPriority), expectedRule.Priority); err != nil {
			return err
		}
		if err := verifyListenerRuleConditions(actualRule.Conditions, expectedRule.Conditions); err != nil {
			return err
		}
		if err := verifyListenerRuleActions(actualRule.Actions, expectedRule.Actions); err != nil {
			return err
		}
	}
	return nil
}

func verifyListenerRulePriority(rulePriority int32, expectedPriority int32) error {
	if rulePriority != expectedPriority {
		return errors.Errorf("expected listener rule priority %d, got %d", expectedPriority, rulePriority)
	}
	return nil
}

func verifyListenerRuleConditions(actual, expected []elbv2types.RuleCondition) error {
	if len(actual) != len(expected) {
		return errors.Errorf("expected %d listener rule conditions, got %d", len(expected), len(actual))
	}

	// Create map of expected key-value pairs for lookup
	expectedMap := make(map[string]string)
	expectedMapForHeaders := make(map[string]string)
	for _, expectedCondition := range expected {
		expectedField := awssdk.ToString(expectedCondition.Field)
		if expectedField == string(elbv2model.RuleConditionFieldQueryString) {
			for _, expectedKV := range expectedCondition.QueryStringConfig.Values {
				expectedMap[awssdk.ToString(expectedKV.Key)] = awssdk.ToString(expectedKV.Value)
			}
		}
		if expectedField == string(elbv2model.RuleConditionFieldHTTPHeader) {
			expectedMapForHeaders[awssdk.ToString(expectedCondition.HttpHeaderConfig.HttpHeaderName)] = expectedCondition.HttpHeaderConfig.Values[0]
		}
	}

	for _, expectedCondition := range expected {
		expectedField := awssdk.ToString(expectedCondition.Field)
		switch expectedField {
		case string(elbv2model.RuleConditionFieldPathPattern):
			var foundPath bool
			for _, actualCondition := range actual {
				if awssdk.ToString(actualCondition.Field) == string(elbv2model.RuleConditionFieldPathPattern) {
					foundPath = true
					if !slices.Equal(actualCondition.PathPatternConfig.Values, expectedCondition.PathPatternConfig.Values) {
						return errors.Errorf("expected listener rule condition path-pattern values %v, got %v", expectedCondition.PathPatternConfig.Values, actualCondition.PathPatternConfig.Values)
					}
				}
			}
			if !foundPath {
				return errors.Errorf("expected listener rule condition with path-pattern field, but not found in actual condition.")
			}
		case string(elbv2model.RuleConditionFieldQueryString):
			var foundQuery bool
			for _, actualCondition := range actual {
				if awssdk.ToString(actualCondition.Field) == string(elbv2model.RuleConditionFieldQueryString) {
					foundQuery = true
					// Check if all expected keys were found
					if len(actualCondition.QueryStringConfig.Values) != len(expectedCondition.QueryStringConfig.Values) {
						return errors.Errorf("expected %d query-string pairs, got %d", len(expectedCondition.QueryStringConfig.Values), len(actualCondition.QueryStringConfig.Values))
					}

					// Check each actual key-value pair
					for _, actualKV := range actualCondition.QueryStringConfig.Values {
						actualKey := awssdk.ToString(actualKV.Key)
						actualValue := awssdk.ToString(actualKV.Value)

						expectedValue, exists := expectedMap[actualKey]
						if !exists {
							return errors.Errorf("unexpected query-string key %v found in actual condition", actualKey)
						}
						if actualValue != expectedValue {
							return errors.Errorf("expected listener rule condition query-string value %v for key %v, got %v", expectedValue, actualKey, actualValue)
						}
					}
				}
			}
			if !foundQuery {
				return errors.Errorf("expected listener rule condition with query-string field, but not found in actual condition.")
			}
		case string(elbv2model.RuleConditionFieldHostHeader):
			var foundHost bool
			for _, actualCondition := range actual {
				if awssdk.ToString(actualCondition.Field) == string(elbv2model.RuleConditionFieldHostHeader) {
					foundHost = true
					if !slices.Equal(actualCondition.HostHeaderConfig.Values, expectedCondition.HostHeaderConfig.Values) {
						return errors.Errorf("expected listener rule condition host-header values %v, got %v", expectedCondition.HostHeaderConfig.Values, actualCondition.HostHeaderConfig.Values)
					}
				}
			}
			if !foundHost {
				return errors.Errorf("expected listener rule condition with host-header field, but not found in actual condition.")
			}
		case string(elbv2model.RuleConditionFieldHTTPHeader):
			var foundHeader bool
			for _, actualCondition := range actual {
				if awssdk.ToString(actualCondition.Field) == string(elbv2model.RuleConditionFieldHTTPHeader) {
					foundHeader = true
					actualName := awssdk.ToString(actualCondition.HttpHeaderConfig.HttpHeaderName)
					expectedValue, exists := expectedMapForHeaders[actualName]
					if !exists {
						return errors.Errorf("unexpected http-header name %v found in actual condition", actualName)
					}
					if actualCondition.HttpHeaderConfig.Values[0] != expectedValue {
						return errors.Errorf("expected listener rule condition http-header value %v, got %v", expectedValue, actualCondition.HttpHeaderConfig.Values[0])
					}
				}
			}
			if !foundHeader {
				return errors.Errorf("expected listener rule condition with http-header field, but not found in actual condition.")
			}
		case string(elbv2model.RuleConditionFieldHTTPRequestMethod):
			var foundMethod bool
			for _, actualCondition := range actual {
				if awssdk.ToString(actualCondition.Field) == string(elbv2model.RuleConditionFieldHTTPRequestMethod) {
					foundMethod = true
					if !slices.Equal(actualCondition.HttpRequestMethodConfig.Values, expectedCondition.HttpRequestMethodConfig.Values) {
						return errors.Errorf("expected listener rule condition http-request-method values %v, got %v", expectedCondition.HttpRequestMethodConfig.Values, actualCondition.HttpRequestMethodConfig.Values)
					}
				}
			}
			if !foundMethod {
				return errors.Errorf("expected listener rule condition with http-request-method field, but not found in actual condition.")
			}
		default:
			return errors.Errorf("unknown listener rule condition field %s", expectedField)
		}
	}
	return nil
}

func verifyListenerRuleActions(actual, expected []elbv2types.Action) error {
	if len(actual) != len(expected) {
		return errors.Errorf("expected %d listener rule actions, got %d", len(expected), len(actual))
	}
	for i, expectedAction := range expected {
		actualAction := actual[i]
		if expectedAction.Type != actualAction.Type {
			return errors.Errorf("expected listener rule action type %s, got %s", expectedAction.Type, actualAction.Type)
		}
		switch expectedAction.Type {
		case elbv2types.ActionTypeEnumForward:
			if len(actualAction.ForwardConfig.TargetGroups) != len(expectedAction.ForwardConfig.TargetGroups) {
				return errors.Errorf("expected %d listener rule action target groups, got %d", len(expectedAction.ForwardConfig.TargetGroups), len(actualAction.ForwardConfig.TargetGroups))
			}
			for i, expectedTG := range expectedAction.ForwardConfig.TargetGroups {
				actualTG := actualAction.ForwardConfig.TargetGroups[i]
				// only check weight
				if awssdk.ToInt32(actualTG.Weight) != awssdk.ToInt32(expectedTG.Weight) {
					return errors.Errorf("expected listener rule action target group weight %d, got %d", awssdk.ToInt32(expectedTG.Weight), awssdk.ToInt32(actualTG.Weight))
				}
			}
		default:
			return errors.Errorf("unknown listener rule action type %s", expectedAction.Type)
		}
	}

	return nil
}
