package lb

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
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
			attributes: []*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledKey, "true")},
			output:     MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledKey, "true")}),
		},
		{
			name:       "one default attribute",
			ok:         true,
			attributes: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledKey, "false")},
			output:     MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledKey, "false")}),
		},
		{
			name:       fmt.Sprintf("%v is invalid", AccessLogsS3EnabledKey),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledKey, "falfadssdfdsse")},
		},
		{
			name:       "one invalid attribute",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledKey, "not a bool")},
		},
		{
			name:       "one invalid attribute",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsKey, "not an int")},
		},
		{
			name:       "one invalid attribute, int too big",
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsKey, "999999")},
		},
		{
			name:       fmt.Sprintf("%v is invalid", RoutingHTTP2EnabledKey),
			ok:         false,
			attributes: []*elbv2.LoadBalancerAttribute{attr(RoutingHTTP2EnabledKey, "falfadssdfdsse")},
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
				attr(DeletionProtectionEnabledKey, "true"),
				attr(AccessLogsS3EnabledKey, "true"),
				attr(AccessLogsS3BucketKey, "bucket name"),
				attr(AccessLogsS3PrefixKey, "prefix"),
				attr(IdleTimeoutTimeoutSecondsKey, "45"),
				attr(RoutingHTTP2EnabledKey, "false"),
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
			name: "a and b contain a default AccessLogsS3Bucket value, expect no change",
			a:    MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketKey, "true")}),
			b:    MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketKey, "true")}),
		},
		{
			name: "a contains a non-default AccessLogsS3Bucket, b contains default, no change",
			a:    MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketKey, "some bucket")}),
			b:    MustNewAttributes(nil),
		},
		{
			name: "a and b contain empty defaults, no change",
			a:    MustNewAttributes(nil),
			b:    MustNewAttributes(nil),
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default DeletionProtectionEnabledKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledKey, "true")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(DeletionProtectionEnabledKey, "true")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default AccessLogsS3EnabledKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledKey, "true")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3EnabledKey, "true")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default AccessLogsS3BucketKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketKey, "some bucket")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3BucketKey, "some bucket")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default AccessLogsS3PrefixKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(AccessLogsS3PrefixKey, "some prefix")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(AccessLogsS3PrefixKey, "some prefix")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default IdleTimeoutTimeoutSecondsKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsKey, "999")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsKey, "999")},
		},
		{
			name:      fmt.Sprintf("a contains default, b contains non-default RoutingHTTP2EnabledKey, make a change"),
			a:         MustNewAttributes(nil),
			b:         MustNewAttributes([]*elbv2.LoadBalancerAttribute{attr(RoutingHTTP2EnabledKey, "false")}),
			changeSet: []*elbv2.LoadBalancerAttribute{attr(RoutingHTTP2EnabledKey, "false")},
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
		attr(DeletionProtectionEnabledKey, "false"),
		attr(AccessLogsS3EnabledKey, "false"),
		attr(AccessLogsS3BucketKey, ""),
		attr(AccessLogsS3PrefixKey, ""),
		attr(IdleTimeoutTimeoutSecondsKey, "60"),
		attr(RoutingHTTP2EnabledKey, "true"),
	}
}

func newattr(lbArn string, attrs []*elbv2.LoadBalancerAttribute) *Attributes {
	a := MustNewAttributes(attrs)
	a.LbArn = lbArn
	return a
}

func TestReconcile(t *testing.T) {
	for _, tc := range []struct {
		Name                               string
		Attributes                         *Attributes
		DescribeLoadBalancerAttributesCall *DescribeLoadBalancerAttributesCall
		ModifyLoadBalancerAttributesCall   *ModifyLoadBalancerAttributesCall
		ExpectedError                      error
	}{
		{
			Name:       "Load Balancer doesn't exist",
			Attributes: &Attributes{},
			DescribeLoadBalancerAttributesCall: &DescribeLoadBalancerAttributesCall{
				LbArn:  aws.String(""),
				Output: nil,
				Err:    fmt.Errorf("ERROR STRING"),
			},
			ExpectedError: errors.New("failed to retrieve attributes from ELBV2 in AWS: ERROR STRING"),
		},
		{
			Name:       "default attribute set",
			Attributes: newattr("arn", nil),
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
			Attributes: newattr("arn", []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsKey, "120")}),
			DescribeLoadBalancerAttributesCall: &DescribeLoadBalancerAttributesCall{
				LbArn:  aws.String("arn"),
				Output: &elbv2.DescribeLoadBalancerAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyLoadBalancerAttributesCall: &ModifyLoadBalancerAttributesCall{
				Input: &elbv2.ModifyLoadBalancerAttributesInput{
					LoadBalancerArn: aws.String("arn"),
					Attributes: []*elbv2.LoadBalancerAttribute{
						{
							Key:   aws.String("idle_timeout.timeout_seconds"),
							Value: aws.String("120"),
						},
					},
				},
				Err: nil,
			},
			ExpectedError: nil,
		},
		{
			Name:       "start with default attribute set, API throws an error",
			Attributes: newattr("arn", []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsKey, "120")}),
			DescribeLoadBalancerAttributesCall: &DescribeLoadBalancerAttributesCall{
				LbArn:  aws.String("arn"),
				Output: &elbv2.DescribeLoadBalancerAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyLoadBalancerAttributesCall: &ModifyLoadBalancerAttributesCall{
				Input: &elbv2.ModifyLoadBalancerAttributesInput{
					LoadBalancerArn: aws.String("arn"),
					Attributes: []*elbv2.LoadBalancerAttribute{
						{
							Key:   aws.String("idle_timeout.timeout_seconds"),
							Value: aws.String("120"),
						},
					},
				},
				Err: fmt.Errorf("Something unexpected happened"),
			},
			ExpectedError: errors.New("failed modifying attributes: Something unexpected happened"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			elbv2svc := &mocks.ELBV2API{}
			if tc.DescribeLoadBalancerAttributesCall != nil {
				elbv2svc.On("DescribeLoadBalancerAttributes", &elbv2.DescribeLoadBalancerAttributesInput{LoadBalancerArn: tc.DescribeLoadBalancerAttributesCall.LbArn}).Return(tc.DescribeLoadBalancerAttributesCall.Output, tc.DescribeLoadBalancerAttributesCall.Err)
			}

			if tc.ModifyLoadBalancerAttributesCall != nil {
				elbv2svc.On("ModifyLoadBalancerAttributes", tc.ModifyLoadBalancerAttributesCall.Input).Return(tc.ModifyLoadBalancerAttributesCall.Output, tc.ModifyLoadBalancerAttributesCall.Err)
			}

			controller := NewAttributesController(elbv2svc)
			err := controller.Reconcile(context.Background(), tc.Attributes)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			elbv2svc.AssertExpectations(t)
		})
	}
}
