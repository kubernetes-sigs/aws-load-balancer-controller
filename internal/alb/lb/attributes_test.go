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
		attr(DeletionProtectionEnabledString, "false"),
		attr(AccessLogsS3EnabledString, "false"),
		attr(AccessLogsS3BucketString, ""),
		attr(AccessLogsS3PrefixString, ""),
		attr(IdleTimeoutTimeoutSecondsString, "60"),
		attr(RoutingHTTP2EnabledString, "true"),
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
			Attributes: newattr("arn", []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsString, "120")}),
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
			Attributes: newattr("arn", []*elbv2.LoadBalancerAttribute{attr(IdleTimeoutTimeoutSecondsString, "120")}),
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
