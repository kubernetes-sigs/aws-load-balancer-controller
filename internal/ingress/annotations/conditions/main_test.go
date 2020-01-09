package conditions

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/stretchr/testify/assert"
)

func TestConditionsParse_Valid(t *testing.T) {
	tcs := []struct {
		name               string
		conditionsJSON     string
		expectedConditions []RuleCondition
	}{
		{
			name:           "host-header",
			conditionsJSON: `[{"field": "host-header","HostHeaderConfig": {"Values": ["www.example.com", "*.alb.example.com"]}}]`,
			expectedConditions: []RuleCondition{
				{
					Field: aws.String(FieldHostHeader),
					HostHeaderConfig: &HostHeaderConditionConfig{
						Values: []*string{aws.String("www.example.com"), aws.String("*.alb.example.com")},
					},
				},
			},
		},
		{
			name:           "path-pattern",
			conditionsJSON: `[{"field": "path-pattern","PathPatternConfig": {"Values": ["/*", "/m00nf1sh"]}}]`,
			expectedConditions: []RuleCondition{
				{
					Field: aws.String(FieldPathPattern),
					PathPatternConfig: &PathPatternConditionConfig{
						Values: []*string{aws.String("/*"), aws.String("/m00nf1sh")},
					},
				},
			},
		},
		{
			name:           "http-header",
			conditionsJSON: `[{"field": "http-header","HttpHeaderConfig": {"HttpHeaderName": "header-a", "Values": ["a"]}}, {"field": "http-header","HttpHeaderConfig": {"HttpHeaderName": "header-b", "Values": ["b-1", "b-2"]}}]`,
			expectedConditions: []RuleCondition{
				{
					Field: aws.String(FieldHTTPHeader),
					HttpHeaderConfig: &HttpHeaderConditionConfig{
						HttpHeaderName: aws.String("header-a"),
						Values:         []*string{aws.String("a")},
					},
				},
				{
					Field: aws.String(FieldHTTPHeader),
					HttpHeaderConfig: &HttpHeaderConditionConfig{
						HttpHeaderName: aws.String("header-b"),
						Values:         []*string{aws.String("b-1"), aws.String("b-2")},
					},
				},
			},
		},
		{
			name:           "http-request-method",
			conditionsJSON: `[{"field": "http-request-method", "HttpRequestMethodConfig": {"Values": ["GET", "HEAD"]}}]`,
			expectedConditions: []RuleCondition{
				{
					Field: aws.String(FieldHTTPRequestMethod),
					HttpRequestMethodConfig: &HttpRequestMethodConditionConfig{
						Values: []*string{aws.String("GET"), aws.String("HEAD")},
					},
				},
			},
		},
		{
			name:           "query-string",
			conditionsJSON: `[{"field": "query-string", "QueryStringConfig": {"Values": [{"Key": "user", "Value": "m00n"},{"Key": "eats", "Value": "f1sh"}]}}]`,
			expectedConditions: []RuleCondition{
				{
					Field: aws.String(FieldQueryString),
					QueryStringConfig: &QueryStringConditionConfig{
						Values: []*QueryStringKeyValuePair{
							{
								Key:   aws.String("user"),
								Value: aws.String("m00n"),
							},
							{
								Key:   aws.String("eats"),
								Value: aws.String("f1sh"),
							},
						},
					},
				},
			},
		},
	}

	data := map[string]string{}
	for _, tc := range tcs {
		data[parser.GetAnnotationWithPrefix("conditions."+tc.name)] = tc.conditionsJSON
	}
	ing := dummy.NewIngress()
	ing.SetAnnotations(data)

	conditionsConfigRaw, err := NewParser().Parse(ing)
	if err != nil {
		t.Error(err)
		return
	}
	conditionsConfig, ok := conditionsConfigRaw.(*Config)
	if !ok {
		t.Errorf("expected a Config type")
		return
	}

	for _, tc := range tcs {
		assert.Equal(t, tc.expectedConditions, conditionsConfig.Conditions[tc.name])
	}
}

func TestConditionsParse_Invalid(t *testing.T) {
	for _, tc := range []struct {
		name           string
		conditionsJSON string
		expectedErr    string
	}{
		{
			name:           "should error if HostHeaderConfig absent for host-header condition",
			conditionsJSON: `[{"Field": "host-header"}]`,
			expectedErr:    "missing HostHeaderConfig",
		},
		{
			name:           "should error if PathPatternConfig absent for path-pattern condition",
			conditionsJSON: `[{"Field": "path-pattern"}]`,
			expectedErr:    "missing PathPatternConfig",
		},
		{
			name:           "should error if HttpHeaderConfig absent for http-header condition",
			conditionsJSON: `[{"Field": "http-header"}]`,
			expectedErr:    "missing HttpHeaderConfig",
		},
		{
			name:           "should error if HttpRequestMethodConfig absent for http-request-method condition",
			conditionsJSON: `[{"Field": "http-request-method"}]`,
			expectedErr:    "missing HttpRequestMethodConfig",
		},
		{
			name:           "should error if FieldQueryString absent for query-string condition",
			conditionsJSON: `[{"Field": "query-string"}]`,
			expectedErr:    "missing QueryStringConfig",
		},
		{
			name:           "should error if FieldSourceIP absent for source-ip condition",
			conditionsJSON: `[{"Field": "source-ip"}]`,
			expectedErr:    "missing SourceIpConfig",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ing := dummy.NewIngress()
			data := map[string]string{}
			data[parser.GetAnnotationWithPrefix("conditions.test-condition")] = tc.conditionsJSON
			ing.SetAnnotations(data)
			_, err := NewParser().Parse(ing)
			assert.EqualError(t, err, tc.expectedErr)
		})
	}
}
