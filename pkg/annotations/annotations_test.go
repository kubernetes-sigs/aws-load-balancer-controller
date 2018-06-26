package annotations

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

const clusterName = "testCluster"
const ingressName = "testIngressName"
const ingressNamespace = "test-namespace"

func fakeValidator() FakeValidator {
	return FakeValidator{VpcId: "vpc-1"}
}

func TestParseAnnotations(t *testing.T) {
	vf := NewValidatingAnnotationFactory(&NewValidatingAnnotationFactoryOptions{
		Validator:   FakeValidator{VpcId: "vpc-1"},
		ClusterName: clusterName})
	_, err := vf.ParseAnnotations(&ParseAnnotationsOptions{})
	if err == nil {
		t.Fatalf("ParseAnnotations should not accept nil for annotations")
	}
}

func TestSetSuccessCodes(t *testing.T) {
	var tests = []struct {
		annotations map[string]string
		expected    string
		pass        bool
	}{
		{map[string]string{}, "200", true},
		{map[string]string{successCodesKey: "1"}, "1", true},
		{map[string]string{successCodesAltKey: "1"}, "1", true},
		{map[string]string{successCodesKey: "1"}, "2", false},
		{map[string]string{successCodesAltKey: "1"}, "2", false},
		{map[string]string{successCodesKey: "1", successCodesAltKey: "2"}, "1", true},
	}
	for _, tt := range tests {
		a := &Annotations{}

		err := a.setSuccessCodes(tt.annotations)
		if err != nil && tt.pass {
			t.Errorf("setSuccessCodes(%v): expected %v, errored: %v", tt.annotations, tt.expected, err)
		}
		if err == nil && tt.pass && tt.expected != *a.SuccessCodes {
			t.Errorf("setSuccessCodes(%v): expected %v, actual %v", tt.annotations, tt.expected, *a.SuccessCodes)
		}
		if err == nil && !tt.pass && tt.expected == *a.SuccessCodes {
			t.Errorf("setSuccessCodes(%v): expected %v, actual %v", tt.annotations, tt.expected, *a.SuccessCodes)
		}
	}
}

func TestSetScheme(t *testing.T) {
	var tests = []struct {
		scheme   string
		expected string
		pass     bool
	}{
		{"", "", false},
		{"internal", "internal", true},
		{"internal", "internet-facing", false},
		{"internet-facing", "internal", false},
		{"internet-facing", "internet-facing", true},
	}

	for _, tt := range tests {
		a := &Annotations{}

		err := a.setScheme(map[string]string{schemeKey: tt.scheme}, ingressName, ingressNamespace, fakeValidator())
		if err != nil && tt.pass {
			t.Errorf("setScheme(%v): expected %v, errored: %v", tt.scheme, tt.expected, err)
		}
		if err == nil && tt.pass && tt.expected != *a.Scheme {
			t.Errorf("setScheme(%v): expected %v, actual %v", tt.scheme, tt.expected, *a.Scheme)
		}
		if err == nil && !tt.pass && tt.expected == *a.Scheme {
			t.Errorf("setScheme(%v): expected %v, actual %v", tt.scheme, tt.expected, *a.Scheme)
		}
	}
}

func TestSetIpAddressType(t *testing.T) {
	var tests = []struct {
		ipAddressType string
		expected      string
		pass          bool
	}{
		{"", "ipv4", true}, // ip-address-type has a sane default
		{"/", "", false},
		{"ipv4", "ipv4", true},
		{"ipv4", "dualstack", false},
		{"dualstack", "ipv4", false},
		{"dualstack", "dualstack", true},
	}

	for _, tt := range tests {
		a := &Annotations{}

		err := a.setIPAddressType(map[string]string{ipAddressTypeKey: tt.ipAddressType})
		if err != nil && tt.pass {
			t.Errorf("setIPAddressType(%v): expected %v, actual %v", tt.ipAddressType, tt.pass, err)
		}
		if err == nil && tt.pass && tt.expected != *a.IPAddressType {
			t.Errorf("setIPAddressType(%v): expected %v, actual %v", tt.ipAddressType, tt.expected, *a.IPAddressType)
		}
		if err == nil && !tt.pass && tt.expected == *a.IPAddressType {
			t.Errorf("setIPAddressType(%v): expected %v, actual %v", tt.ipAddressType, tt.expected, *a.IPAddressType)
		}
	}
}

func TestSetIgnoreHostHeader(t *testing.T) {
	var tests = []struct {
		ignoreHostHeader string
		expected         bool
	}{
		{"", false},
		{"invalid_input", false},
		{"0", false},
		{"F", false},
		{"f", false},
		{"FALSE", false},
		{"false", false},
		{"False", false},
		{"1", true},
		{"T", true},
		{"t", true},
		{"TRUE", true},
		{"true", true},
		{"True", true},
	}

	for _, tt := range tests {
		a := &Annotations{}

		if a.setIgnoreHostHeader(map[string]string{ignoreHostHeader: tt.ignoreHostHeader}); *a.IgnoreHostHeader != tt.expected {
			t.Errorf("setIgnoreHostHeader(%v): expected %v, actual %v", tt.ignoreHostHeader, tt.expected, *a.IgnoreHostHeader)
		}
	}
}

func TestSetSslPolicy(t *testing.T) {
	var tests = []struct {
		Annotations map[string]string
		expected    string
		pass        bool
	}{
		{map[string]string{}, "", true}, // ssl policy has a sane default
		{map[string]string{sslPolicyKey: "ELBSecurityPolicy-TLS-1-2-2017-01"}, "", false},
		{map[string]string{certificateArnKey: "arn:aws:acm:"}, "ELBSecurityPolicy-2016-08", true}, // AWS's default policy when there is a cert assigned is 'ELBSecurityPolicy-2016-08'
		{map[string]string{sslPolicyKey: "ELBSecurityPolicy-TLS-1-2-2017-01"}, "ELBSecurityPolicy-TLS-1-2-2017-01", true},
	}

	for _, tt := range tests {
		a := &Annotations{}
		a.setCertificateArn(tt.Annotations, fakeValidator())

		err := a.setSslPolicy(tt.Annotations, fakeValidator())
		if err != nil && tt.pass {
			t.Errorf("setSslPolicy(%v): expected %v, actual %v", tt.Annotations[sslPolicyKey], tt.pass, err)
		}
		if err == nil && tt.pass && a.SslPolicy != nil && tt.expected != *a.SslPolicy {
			t.Errorf("setSslPolicy(%v): expected %v, actual %v", tt.Annotations[sslPolicyKey], tt.expected, *a.SslPolicy)
		}
		if err == nil && !tt.pass && tt.expected == *a.SslPolicy {
			t.Errorf("setSslPolicy(%v): expected %v, actual %v", tt.Annotations[sslPolicyKey], tt.expected, *a.SslPolicy)
		}
	}
}

// Should fail to create due to healthchecktimeout being greater than HealthcheckIntervalSeconds
func TestHealthcheckSecondsValidation(t *testing.T) {
	a := &Annotations{}
	if err := a.setHealthcheckIntervalSeconds(map[string]string{healthcheckIntervalSecondsKey: "5"}); err != nil {
		t.Errorf("Unexpected error seting HealthcheckIntervalSeconds. Error: %s", err.Error())
	}

	if err := a.setHealthcheckTimeoutSeconds(map[string]string{healthcheckTimeoutSecondsKey: "10"}); err == nil {
		t.Errorf("Set healthchecktimeoutSeconds when it should have failed due to being higher than HealthcheckIntervalSeconds")
	}
}

// Should fail when idle timeout is not in range 1-3600. Should succeed otherwise.
func TestConnectionIdleTimeoutValidation(t *testing.T) {
	a := &Annotations{}

	err := a.setConnectionIdleTimeout(map[string]string{connectionIdleTimeoutKey: "15"})
	if err != nil || a.ConnectionIdleTimeout == aws.Int64(0) {
		t.Error("Failed to set connection idle timeout when value was correct.")
	}

	err = a.setConnectionIdleTimeout(map[string]string{connectionIdleTimeoutKey: "3700"})
	if err == nil {
		t.Error("Succeeded setting connection idle timeout when value was incorrect")
	}
}

func TestSetLoadBalancerAttributes(t *testing.T) {
	var tests = []struct {
		annotations map[string]string
		expected    []elbv2.LoadBalancerAttribute
		length      int
		pass        bool
	}{
		{
			map[string]string{loadbalancerAttributesKey: "access_logs.s3.enabled=true"},
			func() []elbv2.LoadBalancerAttribute {
				e := elbv2.LoadBalancerAttribute{}
				e.SetKey("access_logs.s3.enabled")
				e.SetValue("true")
				return []elbv2.LoadBalancerAttribute{e}
			}(),
			1,
			true,
		},
		{
			map[string]string{loadbalancerAttributesAltKey: "access_logs.s3.enabled=true"},
			func() []elbv2.LoadBalancerAttribute {
				e := elbv2.LoadBalancerAttribute{}
				e.SetKey("access_logs.s3.enabled")
				e.SetValue("true")
				return []elbv2.LoadBalancerAttribute{e}
			}(),
			1,
			true,
		},
		{
			map[string]string{
				loadbalancerAttributesKey:    "access_logs.s3.enabled=true",
				loadbalancerAttributesAltKey: "deletion_protection.enabled=true",
			},
			func() []elbv2.LoadBalancerAttribute {
				e := elbv2.LoadBalancerAttribute{}
				e.SetKey("access_logs.s3.enabled")
				e.SetValue("true")
				return []elbv2.LoadBalancerAttribute{e}
			}(),
			1,
			true,
		},
		{
			map[string]string{loadbalancerAttributesKey: "access_logs.s3.enabled=true,deletion_protection.enabled=true"},
			func() (v []elbv2.LoadBalancerAttribute) {
				e := elbv2.LoadBalancerAttribute{}
				e.SetKey("access_logs.s3.enabled")
				e.SetValue("true")
				v = append(v, e)
				e = elbv2.LoadBalancerAttribute{}
				e.SetKey("deletion_protection.enabled")
				e.SetValue("true")
				v = append(v, e)
				return v
			}(),
			2,
			true,
		},
		{
			map[string]string{loadbalancerAttributesKey: "access_logs.s3.enabled=false"},
			func() []elbv2.LoadBalancerAttribute {
				e := elbv2.LoadBalancerAttribute{}
				e.SetKey("access_logs.s3.enabled")
				e.SetValue("true")
				return []elbv2.LoadBalancerAttribute{e}
			}(),
			1,
			false,
		},
	}
	for v, tt := range tests {
		a := &Annotations{}

		err := a.setLoadBalancerAttributes(tt.annotations)
		if err != nil && tt.pass {
			t.Errorf("setLoadBalancerAttributes(%v): expected %v, errored: %v", tt.annotations, tt.expected, err)
		}
		if err != nil && !tt.pass {
			t.Errorf("setLoadBalancerAttributes(%v): should have errored", tt.annotations)
		}
		if err == nil && len(a.LoadBalancerAttributes) != tt.length {
			t.Errorf("setLoadBalancerAttributes(%v): expected %v attributes, actual %v", tt.annotations, tt.length, len(a.LoadBalancerAttributes))
			continue
		}
		for i := range a.LoadBalancerAttributes {
			if tt.pass && (*a.LoadBalancerAttributes[i].Key != *tt.expected[i].Key ||
				*a.LoadBalancerAttributes[i].Value != *tt.expected[i].Value) {
				t.Errorf("setLoadBalancerAttributes(%v): [test %v, attribute %v] passed but did not match (%v != %v) or (%v != %v)",
					tt.annotations, v, i, *a.LoadBalancerAttributes[i].Key, *tt.expected[i].Key,
					*a.LoadBalancerAttributes[i].Value, *tt.expected[i].Value)
			}
		}
	}
}

func TestSetTargetGroupAttributes(t *testing.T) {
	annotations := &Annotations{}
	attributes := map[string]string{targetGroupAttributesKey: "deregistration_delay.timeout_seconds=60,stickiness.enabled=true"}
	err := annotations.setTargetGroupAttributes(attributes)
	if err != nil || len(annotations.TargetGroupAttributes) != 5 {
		t.Errorf("setTargetGroupAttributes - number of attributes incorrect")
	}

	for _, attr := range annotations.TargetGroupAttributes {
		if *attr.Key == "deregistration_delay.timeout_seconds" && *attr.Value != "60" {
			t.Errorf("setTargetGroupAttributes - deregistration_delay.timeout_seconds value did not match")
		}
		if *attr.Key == "stickiness.enabled" && *attr.Value != "true" {
			t.Errorf("setTargetGroupAttributes - stickiness.enabled value did not match")
		}
	}
}
