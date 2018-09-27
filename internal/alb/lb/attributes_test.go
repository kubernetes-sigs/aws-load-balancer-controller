package lb

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func MustNewAttributes(a []*elbv2.LoadBalancerAttribute) *Attributes {
	attr, _ := NewAttributes(a)
	return attr
}

func attr(k, v string) *elbv2.LoadBalancerAttribute {
	return &elbv2.LoadBalancerAttribute{Key: aws.String(k), Value: aws.String(v)}
}

func Test_NewAttributes(t *testing.T) {

	for _, tc := range []struct {
		name       string
		attributes []*elbv2.LoadBalancerAttribute
		output     *Attributes
		ok         bool
	}{
		{
			name:       "one non-default attribute",
			ok:         true,
			attributes: []*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledString, "true")},
			output:     MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledString, "true")}),
		},
		{
			name:       "one default attribute",
			ok:         true,
			attributes: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledString, "false")},
			output:     MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledString, "false")}),
		},
		{
			name:       fmt.Sprintf("%v is invalid", AccessLogsS3EnabledString),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledString, "falfadssdfdsse")},
		},
		{
			name:       "one invalid attribute",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledString, "not a bool")},
		},
		{
			name:       "one invalid attribute",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsString, "not an int")},
		},
		{
			name:       "one invalid attribute, int too big",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsString, "999999")},
		},
		{
			name:       fmt.Sprintf("%v is invalid", RoutingHTTP2EnabledString),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(RoutingHTTP2EnabledString, "falfadssdfdsse")},
		},
		{
			name:       fmt.Sprintf("undefined attribute"),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr("not.real.attribute", "falfadssdfdsse")},
		},
		{
			name: "non-default attributes",
			ok:   true,
			attributes: []*elbv2.LoadBalancerAttribute{
				attr(DeletionProtectionEnabledString, "true"),
				attr(AccessLogsS3EnabledString, "true"),
				attr(AccessLogsS3BucketString, "bucket name"),
				attr(AccessLogsS3PrefixString, "prefix"),
				attr(IdleTimeoutTimeoutSecondsString, "45"),
				attr(RoutingHTTP2EnabledString, "false"),
			},
			output: &Attributes{
				DeletionProtectionEnabled: true,
				AccessLogsS3Enabled:       true,
				AccessLogsS3Bucket:        "bucket name",
				AccessLogsS3Prefix:        "prefix",
				IdleTimeoutTimeoutSeconds: 45,
				RoutingHTTP2Enabled:       false,
			},
		},
	} {
		output, err := NewAttributes(tc.attributes)

		if tc.ok && err != nil {
			t.Errorf("%v: unexpected error: %v", tc.name, err)
		}

		if !tc.ok && err == nil {
			t.Errorf("%v: expected an error", tc.name)
		}

		if !reflect.DeepEqual(tc.output, output) && err == nil {
			t.Errorf("%v: expected %v, actual %v", tc.name, tc.output, output)
		}
	}
}

func Test_attributesChangeSet(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ok        bool
		a         *Attributes
		b         *Attributes
		changeSet []*elbv2.LoadBalancerAttribute
	}{
		{
			name: "both contain non-default, no change",
			ok:   false,
			a:    MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketString, "true")}),
			b:    MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketString, "true")}),
		},
		{
			name: "a contains non-default, b contains default, no change",
			ok:   false,
			a:    MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketString, "some bucket")}),
			b:    MustNewAttributes(nil),
		},
		{
			name: "a contains default, b contains default, no change",
			ok:   false,
			a:    MustNewAttributes(nil),
			b:    MustNewAttributes(nil),
		},
		{
			name:      fmt.Sprintf("a contains default, b contains %v, make a change", DeletionProtectionEnabledString),
			ok:        true,
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledString, "true")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledString, "true")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains %v, make a change", AccessLogsS3EnabledString),
			ok:        true,
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledString, "true")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledString, "true")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains %v, make a change", AccessLogsS3BucketString),
			ok:        true,
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketString, "some bucket")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketString, "some bucket")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains %v, make a change", AccessLogsS3PrefixString),
			ok:        true,
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3PrefixString, "some prefix")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3PrefixString, "some prefix")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains %v, make a change", IdleTimeoutTimeoutSecondsString),
			ok:        true,
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsString, "999")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsString, "999")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains %v, make a change", RoutingHTTP2EnabledString),
			ok:        true,
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(RoutingHTTP2EnabledString, "false")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(RoutingHTTP2EnabledString, "false")},
		},
	} {

		changeSet, ok := attributesChangeSet(tc.a, tc.b)
		if ok != tc.ok {
			t.Errorf("%v: expected ok to be %v, got %v", tc.name, tc.ok, ok)
		}
		if len(changeSet) != len(tc.changeSet) {
			t.Errorf("%v: expected %v changes, got %v", tc.name, len(tc.changeSet), len(changeSet))
		}
		if !reflect.DeepEqual(tc.changeSet, changeSet) {
			t.Errorf("%v: expected %v, actual %v", tc.name, tc.changeSet, changeSet)
		}
	}
}
