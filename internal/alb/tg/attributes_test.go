package tg

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func MustNewAttributes(a []*elbv2.TargetGroupAttribute) *Attributes {
	attr, _ := NewAttributes(a)
	return attr
}

func Test_NewAttributes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		attributes []*elbv2.TargetGroupAttribute
		output     *Attributes
		ok         bool
	}{
		{
			name:       "DeregistrationDelayTimeoutSecondsKey is default",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300")}),
		},
		{
			name:       "DeregistrationDelayTimeoutSecondsKey is > 3600",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "3601")},
		},
		{
			name:       "DeregistrationDelayTimeoutSecondsKey is < 0",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "-1")},
		},
		{
			name:       "DeregistrationDelayTimeoutSecondsKey is 450",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "450")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "450")}),
		},
		{
			name:       "DeregistrationDelayTimeoutSecondsKey is not a number",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "error")},
		},

		{
			name:       "SlowStartDurationSecondsKey is default",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "0")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "0")}),
		},
		{
			name:       "SlowStartDurationSecondsKey is > 900",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "901")},
		},
		{
			name:       "SlowStartDurationSecondsKey is < 30",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "29")},
		},
		{
			name:       "SlowStartDurationSecondsKey is 45",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "45")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "45")}),
		},
		{
			name:       "SlowStartDurationSecondsKey is not a number",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "error")},
		},

		{
			name:       "StickinessEnabledKey is default",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "false")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "false")}),
		},
		{
			name:       "StickinessEnabledKey is true",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "true")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "true")}),
		},
		{
			name:       "StickinessEnabledKey is non-bool",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "not a bool")},
		},

		{
			name:       "StickinessTypeKey is default",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "lb_cookie")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "lb_cookie")}),
		},
		{
			name:       "StickinessTypeKey is not lb_cookie",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "not lb_cookie")},
		},

		{
			name:       "StickinessLbCookieDurationSecondsKey is default",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "86400")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "86400")}),
		},
		{
			name:       "StickinessLbCookieDurationSecondsKey is > 604800",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "604801")},
		},
		{
			name:       "StickinessLbCookieDurationSecondsKey is < 1",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "0")},
		},
		{
			name:       "StickinessLbCookieDurationSecondsKey is 45",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "45")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "45")}),
		},
		{
			name:       "StickinessLbCookieDurationSecondsKey is not a number",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "error")},
		},

		{
			name:       "LoadBalancingAlgorithmTypeKey is default",
			ok:         true,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "round_robin")},
			output:     MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "round_robin")}),
		},
		{
			name:       "LoadBalancingAlgorithmTypeKey is not round_robin or least_outstanding_requests",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "error")},
		},

		{
			name:       "Invalid attribute",
			ok:         false,
			attributes: []*elbv2.TargetGroupAttribute{tgAttribute("not a real attribute", "error")},
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

			if err == nil {
				assert.Equal(t, tc.output, output, "expected %v, actual %v", tc.output, output)
			}
		})
	}
}

func Test_attributesChangeSet(t *testing.T) {
	for _, tc := range []struct {
		name      string
		a         *Attributes
		b         *Attributes
		changeSet []*elbv2.TargetGroupAttribute
	}{
		{
			name: "DeregistrationDelayTimeoutSeconds: a=default b=default",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300")}),
		},
		{
			name:      "DeregistrationDelayTimeoutSeconds: a=nondefault b=default",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "500")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300")},
		},
		{
			name:      "DeregistrationDelayTimeoutSeconds: a=default b=nondefault",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "500")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "500")},
		},
		{
			name: "DeregistrationDelayTimeoutSeconds: a=nondefault b=nondefault a=b",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "500")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "500")}),
		},
		{
			name:      "DeregistrationDelayTimeoutSeconds: a=nondefault b=nondefault a!=b",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "500")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "501")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(DeregistrationDelayTimeoutSecondsKey, "501")},
		},

		{
			name: "SlowStartDurationSeconds: a=default b=default",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "0")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "0")}),
		},
		{
			name:      "SlowStartDurationSeconds: a=nondefault b=default",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "0")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "0")},
		},
		{
			name:      "SlowStartDurationSeconds: a=default b=nondefault",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "0")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")},
		},
		{
			name: "SlowStartDurationSeconds: a=nondefault b=nondefault a=b",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")}),
		},
		{
			name:      "SlowStartDurationSeconds: a=nondefault b=nondefault a!=b",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "501")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "501")},
		},

		{
			name: "StickinessEnabled: a=default b=default",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "false")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "false")}),
		},
		{
			name:      "StickinessEnabled: a=nondefault b=default",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "true")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "false")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "false")},
		},
		{
			name:      "StickinessEnabled: a=default b=nondefault",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "false")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "true")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "true")},
		},
		{
			name: "StickinessEnabled: a=nondefault b=nondefault a=b",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "true")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessEnabledKey, "true")}),
		},

		{
			name: "StickinessType: a=default b=default",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "lb_cookie")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "lb_cookie")}),
		},
		{
			name:      "StickinessType: a=nondefault b=default",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "lb_cookie")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessTypeKey, "lb_cookie")},
		},

		{
			name: "StickinessLbCookieDurationSeconds: a=default b=default",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "86400")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "86400")}),
		},
		{
			name:      "StickinessLbCookieDurationSeconds: a=nondefault b=default",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "500")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "86400")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "86400")},
		},
		{
			name:      "StickinessLbCookieDurationSeconds: a=default b=nondefault",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "86400")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "500")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "500")},
		},
		{
			name: "StickinessLbCookieDurationSeconds: a=nondefault b=nondefault a=b",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "500")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "500")}),
		},
		{
			name:      "StickinessLbCookieDurationSeconds: a=nondefault b=nondefault a!=b",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "500")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "501")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(StickinessLbCookieDurationSecondsKey, "501")},
		},

		{
			name: "LoadBalancingAlgorithmType: a=default b=default",
			a:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "round_robin")}),
			b:    MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "round_robin")}),
		},
		{
			name:      "LoadBalancingAlgorithmType: a=default b=nondefault a!=b",
			a:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "round_robin")}),
			b:         MustNewAttributes([]*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "least_outstanding_requests")}),
			changeSet: []*elbv2.TargetGroupAttribute{tgAttribute(LoadBalancingAlgorithmTypeKey, "least_outstanding_requests")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changeSet := attributesChangeSet(tc.a, tc.b)
			assert.Equal(t, len(changeSet), len(tc.changeSet), "expected %v changes", len(tc.changeSet))
			assert.Equal(t, tc.changeSet, changeSet, "expected changes")
		})
	}
}

type DescribeTargetGroupAttributesCall struct {
	TgArn  *string
	Output *elbv2.DescribeTargetGroupAttributesOutput
	Err    error
}

type ModifyTargetGroupAttributesCall struct {
	Input  *elbv2.ModifyTargetGroupAttributesInput
	Output *elbv2.ModifyTargetGroupAttributesOutput
	Err    error
}

func defaultAttributes() []*elbv2.TargetGroupAttribute {
	return []*elbv2.TargetGroupAttribute{
		tgAttribute(DeregistrationDelayTimeoutSecondsKey, "300"),
		tgAttribute(SlowStartDurationSecondsKey, "0"),
		tgAttribute(StickinessEnabledKey, "false"),
		tgAttribute(StickinessTypeKey, "lb_cookie"),
		tgAttribute(StickinessLbCookieDurationSecondsKey, "86400"),
	}
}

func Test_AttributesReconcile(t *testing.T) {
	for _, tc := range []struct {
		Name                              string
		Attributes                        []*elbv2.TargetGroupAttribute
		DescribeTargetGroupAttributesCall *DescribeTargetGroupAttributesCall
		ModifyTargetGroupAttributesCall   *ModifyTargetGroupAttributesCall
		ExpectedError                     error
	}{
		{
			Name:       "Target Group doesn't exist",
			Attributes: nil,
			DescribeTargetGroupAttributesCall: &DescribeTargetGroupAttributesCall{
				TgArn:  aws.String("arn"),
				Output: nil,
				Err:    fmt.Errorf("ERROR STRING"),
			},
			ExpectedError: errors.New("failed to retrieve attributes from TargetGroup in AWS: ERROR STRING"),
		},
		{
			Name:       "default attribute set",
			Attributes: nil,
			DescribeTargetGroupAttributesCall: &DescribeTargetGroupAttributesCall{
				TgArn:  aws.String("arn"),
				Output: &elbv2.DescribeTargetGroupAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyTargetGroupAttributesCall: nil,
			ExpectedError:                   nil,
		},
		{
			Name:       "start with default attribute set, change SlowStartDurationSecondsKey to 500s",
			Attributes: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")},
			DescribeTargetGroupAttributesCall: &DescribeTargetGroupAttributesCall{
				TgArn:  aws.String("arn"),
				Output: &elbv2.DescribeTargetGroupAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyTargetGroupAttributesCall: &ModifyTargetGroupAttributesCall{
				Input: &elbv2.ModifyTargetGroupAttributesInput{
					TargetGroupArn: aws.String("arn"),
					Attributes: []*elbv2.TargetGroupAttribute{
						tgAttribute("slow_start.duration_seconds", "500"),
					},
				},
				Err: nil,
			},
			ExpectedError: nil,
		},
		{
			Name:       "start with default attribute set, API throws an error",
			Attributes: []*elbv2.TargetGroupAttribute{tgAttribute(SlowStartDurationSecondsKey, "500")},
			DescribeTargetGroupAttributesCall: &DescribeTargetGroupAttributesCall{
				TgArn:  aws.String("arn"),
				Output: &elbv2.DescribeTargetGroupAttributesOutput{Attributes: defaultAttributes()},
				Err:    nil,
			},
			ModifyTargetGroupAttributesCall: &ModifyTargetGroupAttributesCall{
				Input: &elbv2.ModifyTargetGroupAttributesInput{
					TargetGroupArn: aws.String("arn"),
					Attributes: []*elbv2.TargetGroupAttribute{
						tgAttribute("slow_start.duration_seconds", "500"),
					},
				},
				Err: fmt.Errorf("Something unexpected happened"),
			},
			ExpectedError: errors.New("Something unexpected happened"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.DescribeTargetGroupAttributesCall != nil {
				cloud.On("DescribeTargetGroupAttributesWithContext", ctx, &elbv2.DescribeTargetGroupAttributesInput{TargetGroupArn: tc.DescribeTargetGroupAttributesCall.TgArn}).Return(tc.DescribeTargetGroupAttributesCall.Output, tc.DescribeTargetGroupAttributesCall.Err)
			}

			if tc.ModifyTargetGroupAttributesCall != nil {
				cloud.On("ModifyTargetGroupAttributesWithContext", ctx, tc.ModifyTargetGroupAttributesCall.Input).Return(tc.ModifyTargetGroupAttributesCall.Output, tc.ModifyTargetGroupAttributesCall.Err)
			}

			controller := NewAttributesController(cloud)
			err := controller.Reconcile(context.Background(), "arn", tc.Attributes)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			cloud.AssertExpectations(t)
		})
	}
}
