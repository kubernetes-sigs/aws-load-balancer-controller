package action

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
)

func TestIngressActions(t *testing.T) {
	tcs := []struct {
		name           string
		actionJSON     string
		expectedAction Action
	}{
		{
			name: "fixed-response-action",
			actionJSON: `{"Type": "fixed-response", "FixedResponseConfig": {"ContentType":"text/plain",
	"StatusCode":"503", "MessageBody":"message body"}}`,
			expectedAction: Action{
				Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
				FixedResponseConfig: &FixedResponseActionConfig{
					ContentType: aws.String("text/plain"),
					MessageBody: aws.String("message body"),
					StatusCode:  aws.String("503"),
				},
			},
		},
		{
			name: "redirect-action",
			actionJSON: `{"Type": "redirect", "RedirectConfig": {"Protocol":"HTTPS",
  "Port":"443", "StatusCode": "HTTP_301"}}`,
			expectedAction: Action{
				Type: aws.String(elbv2.ActionTypeEnumRedirect),
				RedirectConfig: &RedirectActionConfig{
					Protocol:   aws.String("HTTPS"),
					Port:       aws.String("443"),
					Host:       aws.String("#{host}"),
					Path:       aws.String("/#{path}"),
					Query:      aws.String("#{query}"),
					StatusCode: aws.String("HTTP_301"),
				},
			},
		},
		{
			name:       "forward",
			actionJSON: `{"Type": "forward", "TargetGroupArn": "legacy-tg-arn"}`,
			expectedAction: Action{
				Type: aws.String(elbv2.ActionTypeEnumForward),
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []*TargetGroupTuple{
						{
							TargetGroupArn: aws.String("legacy-tg-arn"),
						},
					},
				},
				TargetGroupArn: aws.String("legacy-tg-arn"),
			},
		},
		{
			name:       "forward-weighted",
			actionJSON: `{"Type": "forward", "ForwardConfig": {"TargetGroups": [{"TargetGroupArn": "legacy-tg-arn", "weight": 10}, {"ServiceName": "svc", "ServicePort": "https", "weight": 20}], "TargetGroupStickinessConfig": {"Enabled": true, "DurationSeconds": 100}}}`,
			expectedAction: Action{
				Type: aws.String(elbv2.ActionTypeEnumForward),
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []*TargetGroupTuple{
						{
							TargetGroupArn: aws.String("legacy-tg-arn"),
							Weight:         aws.Int64(10),
						},
						{
							ServiceName: aws.String("svc"),
							ServicePort: aws.String("https"),
							Weight:      aws.Int64(20),
						},
					},
					TargetGroupStickinessConfig: &TargetGroupStickinessConfig{
						DurationSeconds: aws.Int64(100),
						Enabled:         aws.Bool(true),
					},
				},
			},
		},
	}

	data := map[string]string{}
	for _, tc := range tcs {
		data[parser.GetAnnotationWithPrefix("actions."+tc.name)] = tc.actionJSON
	}
	ing := dummy.NewIngress()
	ing.SetAnnotations(data)

	actionsConfigRaw, err := NewParser().Parse(ing)
	if err != nil {
		t.Error(err)
		return
	}
	actionsConfig, ok := actionsConfigRaw.(*Config)
	if !ok {
		t.Errorf("expected a Config type")
		return
	}
	for _, tc := range tcs {
		assert.Equal(t, tc.expectedAction, actionsConfig.Actions[tc.name])
	}
}

func TestInvalidIngressActions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		actionJSON  string
		expectedErr string
	}{
		{
			name:        "should error if FixedResponseConfig absent for fixed-response action",
			actionJSON:  `{"Type": "fixed-response"}`,
			expectedErr: "missing FixedResponseConfig",
		},
		{
			name:        "should error if RedirectConfig absent for redirect action",
			actionJSON:  `{"Type": "redirect"}`,
			expectedErr: "missing RedirectConfig",
		},
		{
			name:        "should error if StatusCode absent for RedirectConfig",
			actionJSON:  `{"Type": "redirect", "RedirectConfig": {"Host": "#{host}"}}`,
			expectedErr: "invalid RedirectConfig: StatusCode is required",
		},
		{
			name:        "should error if both TargetGroupArn and ForwardConfig absent for for forward action",
			actionJSON:  `{"Type": "forward"}`,
			expectedErr: "precisely one of TargetGroupArn and ForwardConfig can be specified",
		},
		{
			name:        "should error if both TargetGroupArn and ForwardConfig are specified for forward action ",
			actionJSON:  `{"Type": "forward", "TargetGroupArn": "tg-1", "ForwardConfig": {"TargetGroups": [{"TargetGroupArn": "tg-2", "weight": 10}]}}`,
			expectedErr: "precisely one of TargetGroupArn and ForwardConfig can be specified",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ing := dummy.NewIngress()
			data := map[string]string{}
			data[parser.GetAnnotationWithPrefix("actions.test-action")] = tc.actionJSON
			ing.SetAnnotations(data)
			_, err := NewParser().Parse(ing)
			assert.EqualError(t, err, tc.expectedErr)
		})
	}
}
