package rs

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_NewRules(t *testing.T) {
	for _, tc := range []struct {
		Name    string
		Ingress *extensions.Ingress
	}{
		{
			Name:    "std params",
			Ingress: &extensions.Ingress{},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			output := NewRules(tc.Ingress)

			assert.Equal(t, output.Ingress, tc.Ingress)
		})
	}
}

func Test_priority(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Value    string
		Expected int64
	}{
		{
			Name:     "valid integer priority",
			Value:    "3",
			Expected: 3,
		},
		{
			Name:     "invalid priority",
			Value:    "invalid",
			Expected: 0,
		},
		{
			Name:     "valid default priority",
			Value:    "default",
			Expected: 0,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			o := priority(aws.String(tc.Value))
			assert.Equal(t, aws.Int64Value(o), tc.Expected)
		})
	}
}

func Test_condition(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Field    string
		Values   []string
		Expected *elbv2.RuleCondition
	}{
		{
			Name:     "one value",
			Field:    "field name",
			Values:   []string{"val1"},
			Expected: &elbv2.RuleCondition{Field: aws.String("field name"), Values: aws.StringSlice([]string{"val1"})},
		},
		{
			Name:     "three Values",
			Field:    "field name",
			Values:   []string{"val1", "val2", "val3"},
			Expected: &elbv2.RuleCondition{Field: aws.String("field name"), Values: aws.StringSlice([]string{"val1", "val2", "val3"})},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			o := condition(tc.Field, tc.Values...)
			assert.Equal(t, o, tc.Expected)
		})
	}
}

type DescribeRulesCall struct {
	DescribeRulesOutput *elbv2.DescribeRulesOutput
	Error               error
}

type DescribeTagsCall struct {
	DescribeTagsOutput *elbv2.DescribeTagsOutput
	Error              error
}

func newReq(data interface{}, err error) *request.Request {
	return &request.Request{Data: data, Operation: &request.Operation{Paginator: &request.Paginator{}}, Error: err}
}
func tag(k, v string) *elbv2.Tag {
	return &elbv2.Tag{Key: aws.String(k), Value: aws.String(v)}
}

func Test_getCurrentRules(t *testing.T) {
	listenerArn := "listenerArn"
	tgArn := "tgArn"
	for _, tc := range []struct {
		Name              string
		DescribeRulesCall *DescribeRulesCall
		DescribeTagsCall  *DescribeTagsCall
		Expected          []*Rule
		ExpectedError     error
	}{
		{
			Name:              "DescribeRulesRequest throws an error error",
			DescribeRulesCall: &DescribeRulesCall{DescribeRulesOutput: &elbv2.DescribeRulesOutput{}, Error: errors.New("Some error")},
			ExpectedError:     errors.New("Some error"),
		},
		{
			Name: "DescribeRulesRequest returns one rule",
			DescribeRulesCall: &DescribeRulesCall{DescribeRulesOutput: &elbv2.DescribeRulesOutput{Rules: []*elbv2.Rule{
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: aws.String(tgArn),
						},
					},
					Conditions: []*elbv2.RuleCondition{
						condition("path-pattern", "/*"),
					},
				},
			}}},
			DescribeTagsCall: &DescribeTagsCall{DescribeTagsOutput: &elbv2.DescribeTagsOutput{TagDescriptions: []*elbv2.TagDescription{
				{
					ResourceArn: aws.String(tgArn),
					Tags: []*elbv2.Tag{
						tag(tags.ServiceName, "ServiceName"),
						tag(tags.ServicePort, "http"),
					},
				},
			}}},
			Expected: []*Rule{
				{
					Rule: elbv2.Rule{
						Priority: aws.String("1"),
						Actions: []*elbv2.Action{
							{
								Type:           aws.String(elbv2.ActionTypeEnumForward),
								TargetGroupArn: aws.String(tgArn),
							},
						},
						Conditions: []*elbv2.RuleCondition{
							condition("path-pattern", "/*"),
						},
					},
					Backend: extensions.IngressBackend{
						ServiceName: "ServiceName",
						ServicePort: intstr.FromString("http"),
					},
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			elbv2svc := &mocks.ELBV2API{}
			if tc.DescribeRulesCall != nil {
				elbv2svc.On("DescribeRulesRequest",
					&elbv2.DescribeRulesInput{ListenerArn: aws.String(listenerArn)}).Return(newReq(tc.DescribeRulesCall.DescribeRulesOutput, tc.DescribeRulesCall.Error), tc.DescribeRulesCall.DescribeRulesOutput)
			}
			if tc.DescribeTagsCall != nil {
				elbv2svc.On("DescribeTags",
					&elbv2.DescribeTagsInput{ResourceArns: []*string{aws.String(tgArn)}}).Return(tc.DescribeTagsCall.DescribeTagsOutput, tc.DescribeTagsCall.Error)
			}

			store := &mocks.Storer{}
			controller := &rulesController{
				elbv2: elbv2svc,
				store: store,
			}
			results, err := controller.getCurrentRules(listenerArn)
			assert.Equal(t, results, tc.Expected)
			assert.Equal(t, err, tc.ExpectedError)
			elbv2svc.AssertExpectations(t)
		})
	}
}
