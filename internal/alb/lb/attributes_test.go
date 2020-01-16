package lb

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/stretchr/testify/assert"
)

func MustNewAttributes(a []*elbv2.LoadBalancerAttribute) *Attributes {
	attr, _ := NewAttributes(a)
	return attr
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
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(DeletionProtectionEnabledKey, "true")},
			output:     MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(DeletionProtectionEnabledKey, "true")}),
		},
		{
			name:       "one default attribute",
			ok:         true,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "false")},
			output:     MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "false")}),
		},
		{
			name:       fmt.Sprintf("%v is invalid", AccessLogsS3EnabledKey),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "falfadssdfdsse")},
		},
		{
			name:       "one invalid attribute",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(DeletionProtectionEnabledKey, "not a bool")},
		},
		{
			name:       "one invalid attribute",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(IdleTimeoutTimeoutSecondsKey, "not an int")},
		},
		{
			name:       "one invalid attribute, int too big",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(IdleTimeoutTimeoutSecondsKey, "999999")},
		},
		{
			name:       fmt.Sprintf("%v is invalid", RoutingHTTP2EnabledKey),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(RoutingHTTP2EnabledKey, "falfadssdfdsse")},
		},
		{
			name:       fmt.Sprintf("%v is invalid", DropInvalidHeaderFieldsEnabledKey),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(DropInvalidHeaderFieldsEnabledKey, "falfadssdfdsse")},
		},
		{
			name:       fmt.Sprintf("undefined attribute"),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{lbAttribute("not.real.attribute", "falfadssdfdsse")},
		},
		{
			name: "non-default attributes",
			ok:   true,
			attributes: []*elbv2.LoadBalancerAttribute{
				lbAttribute(DeletionProtectionEnabledKey, "true"),
				lbAttribute(AccessLogsS3EnabledKey, "true"),
				lbAttribute(AccessLogsS3BucketKey, "bucket name"),
				lbAttribute(AccessLogsS3PrefixKey, "prefix"),
				lbAttribute(IdleTimeoutTimeoutSecondsKey, "45"),
				lbAttribute(RoutingHTTP2EnabledKey, "false"),
				lbAttribute(DropInvalidHeaderFieldsEnabledKey, "true"),
			},
			output: &Attributes{
				DeletionProtectionEnabled:      true,
				AccessLogsS3Enabled:            true,
				AccessLogsS3Bucket:             "bucket name",
				AccessLogsS3Prefix:             "prefix",
				IdleTimeoutTimeoutSeconds:      45,
				RoutingHTTP2Enabled:            false,
				DropInvalidHeaderFieldsEnabled: true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := NewAttributes(tc.attributes)

			if tc.ok && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.ok && err == nil {
				t.Errorf("expected an error")
			}

			if !reflect.DeepEqual(tc.output, output) && err == nil {
				t.Errorf("expected %v, actual %v", tc.output, output)
			}
		})
	}
}

func Test_attributesChangeSet(t *testing.T) {
	for _, tc := range []struct {
		name      string
		a         *Attributes
		b         *Attributes
		changeSet []*elbv2.LoadBalancerAttribute
	}{
		{
			name: "a and b contain empty defaults, no change",
			a:    MustNewAttributes(nil),
			b:    MustNewAttributes(nil),
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default DeletionProtectionEnabledKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(DeletionProtectionEnabledKey, "true")}),
			changeSet: []*elbv2.LoadBalancerAttribute{lbAttribute(DeletionProtectionEnabledKey, "true")},
		},
		{
			name:      fmt.Sprintf("enable AccessLogS3"),
			a:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "false"), lbAttribute(AccessLogsS3BucketKey, ""), lbAttribute(AccessLogsS3PrefixKey, "")}),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "true"), lbAttribute(AccessLogsS3BucketKey, "bucket"), lbAttribute(AccessLogsS3PrefixKey, "prefix")}),
			changeSet: []*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "true"), lbAttribute(AccessLogsS3BucketKey, "bucket"), lbAttribute(AccessLogsS3PrefixKey, "prefix")},
		},
		{
			name:      fmt.Sprintf("disable AccessLogS3, don't change bucket/prefix when it's unchanged."),
			a:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "true"), lbAttribute(AccessLogsS3BucketKey, "bucket"), lbAttribute(AccessLogsS3PrefixKey, "prefix")}),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "false"), lbAttribute(AccessLogsS3BucketKey, "bucket"), lbAttribute(AccessLogsS3PrefixKey, "prefix")}),
			changeSet: []*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "false")},
		},
		{
			name:      fmt.Sprintf("disable AccessLogS3, don't change bucket/prefix when it's changed"),
			a:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "true"), lbAttribute(AccessLogsS3BucketKey, "bucket"), lbAttribute(AccessLogsS3PrefixKey, "prefix")}),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "false"), lbAttribute(AccessLogsS3BucketKey, ""), lbAttribute(AccessLogsS3PrefixKey, "")}),
			changeSet: []*elbv2.LoadBalancerAttribute{lbAttribute(AccessLogsS3EnabledKey, "false")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default IdleTimeoutTimeoutSecondsKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(IdleTimeoutTimeoutSecondsKey, "999")}),
			changeSet: []*elbv2.LoadBalancerAttribute{lbAttribute(IdleTimeoutTimeoutSecondsKey, "999")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default RoutingHTTP2EnabledKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(RoutingHTTP2EnabledKey, "false")}),
			changeSet: []*elbv2.LoadBalancerAttribute{lbAttribute(RoutingHTTP2EnabledKey, "false")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default DropInvalidHeaderFieldsEnabledKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{lbAttribute(DropInvalidHeaderFieldsEnabledKey, "true")}),
			changeSet: []*elbv2.LoadBalancerAttribute{lbAttribute(DropInvalidHeaderFieldsEnabledKey, "true")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changeSet := attributesChangeSet(tc.a, tc.b)
			if len(changeSet) != len(tc.changeSet) {
				assert.Equal(t, len(changeSet), len(tc.changeSet), "expected %v changes", len(tc.changeSet))
			}
			if !reflect.DeepEqual(tc.changeSet, changeSet) {
				assert.Equal(t, tc.changeSet, changeSet, "expected changes")
			}
		})
	}
}

type DescribeLoadBalancerAttributesCall struct {
	LbArn  *string
	Output *elbv2.DescribeLoadBalancerAttributesOutput
	Err    error
}

type ModifyLoadBalancerAttributesCall struct {
	Input  *elbv2.ModifyLoadBalancerAttributesInput
	Output *elbv2.ModifyLoadBalancerAttributesOutput
	Err    error
}

func defaultAttributes() []*elbv2.LoadBalancerAttribute {
	return []*elbv2.LoadBalancerAttribute{
		lbAttribute(DeletionProtectionEnabledKey, "false"),
		lbAttribute(AccessLogsS3EnabledKey, "false"),
		lbAttribute(AccessLogsS3BucketKey, ""),
		lbAttribute(AccessLogsS3PrefixKey, ""),
		lbAttribute(IdleTimeoutTimeoutSecondsKey, "60"),
		lbAttribute(RoutingHTTP2EnabledKey, "true"),
		lbAttribute(DropInvalidHeaderFieldsEnabledKey, "false"),
	}
}

func TestReconcile(t *testing.T) {
	lbArn := "arn"
	for _, tc := range []struct {
		Name                               string
		Attributes                         []*elbv2.LoadBalancerAttribute
		DescribeLoadBalancerAttributesCall *DescribeLoadBalancerAttributesCall
		ModifyLoadBalancerAttributesCall   *ModifyLoadBalancerAttributesCall
		ExpectedError                      error
	}{
		{
			Name:       "Load Balancer doesn't exist",
			Attributes: nil,
			DescribeLoadBalancerAttributesCall: &DescribeLoadBalancerAttributesCall{
				LbArn:  aws.String(lbArn),
				Output: nil,
				Err:    fmt.Errorf("ERROR STRING"),
			},
			ExpectedError: errors.New("failed to retrieve attributes from ELBV2 in AWS: ERROR STRING"),
		},
		{
			Name:       "default attribute set",
			Attributes: nil,
			DescribeLoadBalancerAttributesCall: &DescribeLoadBalancerAttributesCall{
				LbArn:  aws.String("arn"),
				Output: &elbv2.DescribeLoadBalancerAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyLoadBalancerAttributesCall: nil,
			ExpectedError:                    nil,
		},
		{
			Name:       "start with default attribute set, change to timeout to 120s",
			Attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(IdleTimeoutTimeoutSecondsKey, "120")},
			DescribeLoadBalancerAttributesCall: &DescribeLoadBalancerAttributesCall{
				LbArn:  aws.String("arn"),
				Output: &elbv2.DescribeLoadBalancerAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyLoadBalancerAttributesCall: &ModifyLoadBalancerAttributesCall{
				Input: &elbv2.ModifyLoadBalancerAttributesInput{
					LoadBalancerArn: aws.String("arn"),
					Attributes: []*elbv2.LoadBalancerAttribute{
						lbAttribute("idle_timeout.timeout_seconds", "120"),
					},
				},
				Err: nil,
			},
			ExpectedError: nil,
		},
		{
			Name:       "start with default attribute set, API throws an error",
			Attributes: []*elbv2.LoadBalancerAttribute{lbAttribute(IdleTimeoutTimeoutSecondsKey, "120")},
			DescribeLoadBalancerAttributesCall: &DescribeLoadBalancerAttributesCall{
				LbArn:  aws.String("arn"),
				Output: &elbv2.DescribeLoadBalancerAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyLoadBalancerAttributesCall: &ModifyLoadBalancerAttributesCall{
				Input: &elbv2.ModifyLoadBalancerAttributesInput{
					LoadBalancerArn: aws.String("arn"),
					Attributes: []*elbv2.LoadBalancerAttribute{
						lbAttribute("idle_timeout.timeout_seconds", "120"),
					},
				},
				Err: fmt.Errorf("Something unexpected happened"),
			},
			ExpectedError: errors.New("failed modifying attributes: Something unexpected happened"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.DescribeLoadBalancerAttributesCall != nil {
				cloud.On("DescribeLoadBalancerAttributesWithContext", ctx, &elbv2.DescribeLoadBalancerAttributesInput{LoadBalancerArn: tc.DescribeLoadBalancerAttributesCall.LbArn}).Return(tc.DescribeLoadBalancerAttributesCall.Output, tc.DescribeLoadBalancerAttributesCall.Err)
			}

			if tc.ModifyLoadBalancerAttributesCall != nil {
				cloud.On("ModifyLoadBalancerAttributesWithContext", ctx, tc.ModifyLoadBalancerAttributesCall.Input).Return(tc.ModifyLoadBalancerAttributesCall.Output, tc.ModifyLoadBalancerAttributesCall.Err)
			}

			controller := NewAttributesController(cloud)
			err := controller.Reconcile(context.Background(), lbArn, tc.Attributes)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			cloud.AssertExpectations(t)
		})
	}
}
