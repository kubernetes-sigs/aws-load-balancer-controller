package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"
)

func Test_defaultEnhancedBackendBuilder_Build(t *testing.T) {
	type args struct {
		ing     *networking.Ingress
		backend networking.IngressBackend
	}
	portHTTP := intstr.FromString("http")
	tests := []struct {
		name    string
		args    args
		want    EnhancedBackend
		wantErr error
	}{
		{
			name: "vanilla serviceBackend",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					ServiceName: "my-svc",
					ServicePort: portHTTP,
				},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("my-svc"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
			},
		},
		{
			name: "vanilla serviceBackend with additional conditions",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/conditions.my-svc": `[{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue1", "HeaderValue2"]}}]`,
						},
					},
				},
				backend: networking.IngressBackend{
					ServiceName: "my-svc",
					ServicePort: portHTTP,
				},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("my-svc"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldHTTPHeader,
						HTTPHeaderConfig: &HTTPHeaderConditionConfig{
							HTTPHeaderName: "HeaderName",
							Values:         []string{"HeaderValue1", "HeaderValue2"},
						},
					},
				},
			},
		},
		{
			name: "annotation-based serviceBackend",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/actions.fake-my-svc": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"my-svc","servicePort":"http"}]}}`,
						},
					},
				},
				backend: networking.IngressBackend{
					ServiceName: "fake-my-svc",
					ServicePort: intstr.FromString("use-annotation"),
				},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("my-svc"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
			},
		},
		{
			name: "annotation-based with additional conditions",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/actions.fake-my-svc":    `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"my-svc","servicePort":"http"}]}}`,
							"alb.ingress.kubernetes.io/conditions.fake-my-svc": `[{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue1", "HeaderValue2"]}}]`,
						},
					},
				},
				backend: networking.IngressBackend{
					ServiceName: "fake-my-svc",
					ServicePort: intstr.FromString("use-annotation"),
				},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("my-svc"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldHTTPHeader,
						HTTPHeaderConfig: &HTTPHeaderConditionConfig{
							HTTPHeaderName: "HeaderName",
							Values:         []string{"HeaderValue1", "HeaderValue2"},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			b := &defaultEnhancedBackendBuilder{
				annotationParser: annotationParser,
			}
			got, err := b.Build(context.Background(), tt.args.ing, tt.args.backend)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got, "diff", cmp.Diff(tt.want, got))
			}
		})
	}
}

func Test_defaultEnhancedBackendBuilder_buildConditions(t *testing.T) {
	type args struct {
		ingAnnotation map[string]string
		svcName       string
	}
	tests := []struct {
		name    string
		args    args
		want    []RuleCondition
		wantErr error
	}{
		{
			name: "host header condition",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path1": `[{"field":"host-header","hostHeaderConfig":{"values":["anno.example.com"]}}]`,
				},
				svcName: "rule-path1",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHostHeader,
					HostHeaderConfig: &HostHeaderConditionConfig{
						Values: []string{"anno.example.com"},
					},
				},
			},
		},
		{
			name: "host header condition - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path1": `[{"Field":"host-header","HostHeaderConfig":{"Values":["anno.example.com"]}}]`,
				},
				svcName: "rule-path1",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHostHeader,
					HostHeaderConfig: &HostHeaderConditionConfig{
						Values: []string{"anno.example.com"},
					},
				},
			},
		},
		{
			name: "path pattern condition",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path2": `[{"field":"path-pattern","pathPatternConfig":{"values":["/anno/path2"]}}]`,
				},
				svcName: "rule-path2",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldPathPattern,
					PathPatternConfig: &PathPatternConditionConfig{
						Values: []string{"/anno/path2"},
					},
				},
			},
		},
		{
			name: "path pattern condition - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path2": `[{"Field":"path-pattern","PathPatternConfig":{"Values":["/anno/path2"]}}]`,
				},
				svcName: "rule-path2",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldPathPattern,
					PathPatternConfig: &PathPatternConditionConfig{
						Values: []string{"/anno/path2"},
					},
				},
			},
		},
		{
			name: "http header condition",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path3": `[{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue1", "HeaderValue2"]}}]`,
				},
				svcName: "rule-path3",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &HTTPHeaderConditionConfig{
						HTTPHeaderName: "HeaderName",
						Values:         []string{"HeaderValue1", "HeaderValue2"},
					},
				},
			},
		},
		{
			name: "http header condition - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path3": `[{"Field":"http-header","HttpHeaderConfig":{"HttpHeaderName": "HeaderName", "Values":["HeaderValue1", "HeaderValue2"]}}]`,
				},
				svcName: "rule-path3",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &HTTPHeaderConditionConfig{
						HTTPHeaderName: "HeaderName",
						Values:         []string{"HeaderValue1", "HeaderValue2"},
					},
				},
			},
		},
		{
			name: "http request method condition",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path4": `[{"field":"http-request-method","httpRequestMethodConfig":{"values":["GET", "HEAD"]}}]`,
				},
				svcName: "rule-path4",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHTTPRequestMethod,
					HTTPRequestMethodConfig: &HTTPRequestMethodConditionConfig{
						Values: []string{"GET", "HEAD"},
					},
				},
			},
		},
		{
			name: "http request method condition - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path4": `[{"Field":"http-request-method","HttpRequestMethodConfig":{"Values":["GET", "HEAD"]}}]`,
				},
				svcName: "rule-path4",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHTTPRequestMethod,
					HTTPRequestMethodConfig: &HTTPRequestMethodConditionConfig{
						Values: []string{"GET", "HEAD"},
					},
				},
			},
		},
		{
			name: "query string condition",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path5": `[{"field":"query-string","queryStringConfig":{"values":[{"key":"paramA","value":"valueA1"},{"key":"paramA","value":"valueA2"}]}}]`,
				},
				svcName: "rule-path5",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldQueryString,
					QueryStringConfig: &QueryStringConditionConfig{
						Values: []QueryStringKeyValuePair{
							{
								Key:   awssdk.String("paramA"),
								Value: "valueA1",
							},
							{
								Key:   awssdk.String("paramA"),
								Value: "valueA2",
							},
						},
					},
				},
			},
		},
		{
			name: "query string condition - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path5": `[{"Field":"query-string","QueryStringConfig":{"Values":[{"Key":"paramA","Value":"valueA1"},{"Key":"paramA","Value":"valueA2"}]}}]`,
				},
				svcName: "rule-path5",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldQueryString,
					QueryStringConfig: &QueryStringConditionConfig{
						Values: []QueryStringKeyValuePair{
							{
								Key:   awssdk.String("paramA"),
								Value: "valueA1",
							},
							{
								Key:   awssdk.String("paramA"),
								Value: "valueA2",
							},
						},
					},
				},
			},
		},
		{
			name: "source IP condition",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path6": `[{"field":"source-ip","sourceIpConfig":{"values":["192.168.0.0/16", "172.16.0.0/16"]}}]`,
				},
				svcName: "rule-path6",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldSourceIP,
					SourceIPConfig: &SourceIPConditionConfig{
						Values: []string{"192.168.0.0/16", "172.16.0.0/16"},
					},
				},
			},
		},
		{
			name: "source IP condition - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path6": `[{"Field":"source-ip","SourceIpConfig":{"Values":["192.168.0.0/16", "172.16.0.0/16"]}}]`,
				},
				svcName: "rule-path6",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldSourceIP,
					SourceIPConfig: &SourceIPConditionConfig{
						Values: []string{"192.168.0.0/16", "172.16.0.0/16"},
					},
				},
			},
		},
		{
			name: "multiple conditions",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path7": `[{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue"]}},{"field":"query-string","queryStringConfig":{"values":[{"key":"paramA","value":"valueA"}]}},{"field":"query-string","queryStringConfig":{"values":[{"key":"paramB","value":"valueB"}]}}]`,
				},
				svcName: "rule-path7",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &HTTPHeaderConditionConfig{
						HTTPHeaderName: "HeaderName",
						Values:         []string{"HeaderValue"},
					},
				},
				{
					Field: RuleConditionFieldQueryString,
					QueryStringConfig: &QueryStringConditionConfig{
						Values: []QueryStringKeyValuePair{
							{
								Key:   awssdk.String("paramA"),
								Value: "valueA",
							},
						},
					},
				},
				{
					Field: RuleConditionFieldQueryString,
					QueryStringConfig: &QueryStringConditionConfig{
						Values: []QueryStringKeyValuePair{
							{
								Key:   awssdk.String("paramB"),
								Value: "valueB",
							},
						},
					},
				},
			},
		},
		{
			name: "multiple conditions - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/conditions.rule-path7": `[{"Field":"http-header","HttpHeaderConfig":{"HttpHeaderName": "HeaderName", "Values":["HeaderValue"]}},{"Field":"query-string","QueryStringConfig":{"Values":[{"Key":"paramA","Value":"valueA"}]}},{"Field":"query-string","QueryStringConfig":{"Values":[{"Key":"paramB","Value":"valueB"}]}}]`,
				},
				svcName: "rule-path7",
			},
			want: []RuleCondition{
				{
					Field: RuleConditionFieldHTTPHeader,
					HTTPHeaderConfig: &HTTPHeaderConditionConfig{
						HTTPHeaderName: "HeaderName",
						Values:         []string{"HeaderValue"},
					},
				},
				{
					Field: RuleConditionFieldQueryString,
					QueryStringConfig: &QueryStringConditionConfig{
						Values: []QueryStringKeyValuePair{
							{
								Key:   awssdk.String("paramA"),
								Value: "valueA",
							},
						},
					},
				},
				{
					Field: RuleConditionFieldQueryString,
					QueryStringConfig: &QueryStringConditionConfig{
						Values: []QueryStringKeyValuePair{
							{
								Key:   awssdk.String("paramB"),
								Value: "valueB",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			b := &defaultEnhancedBackendBuilder{
				annotationParser: annotationParser,
			}
			got, err := b.buildConditions(context.Background(), tt.args.ingAnnotation, tt.args.svcName)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got, "diff", cmp.Diff(tt.want, got))
			}
		})
	}
}

func Test_defaultEnhancedBackendBuilder_buildActionViaAnnotation(t *testing.T) {
	type args struct {
		ingAnnotation map[string]string
		svcName       string
	}

	portHTTP := intstr.FromString("http")
	port80 := intstr.FromInt(80)
	port443 := intstr.FromInt(443)
	_ = port443
	tests := []struct {
		name    string
		args    args
		want    Action
		wantErr error
	}{
		{
			name: "forward action - simplified schema",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.forward-single-tg": `{"type":"forward","targetGroupARN": "tg-arn"}`,
				},
				svcName: "forward-single-tg",
			},
			want: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							TargetGroupARN: awssdk.String("tg-arn"),
						},
					},
				},
			},
		},
		{
			name: "forward action - simplified schema - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.forward-single-tg": `{"Type":"forward","TargetGroupArn": "tg-arn"}`,
				},
				svcName: "forward-single-tg",
			},
			want: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							TargetGroupARN: awssdk.String("tg-arn"),
						},
					},
				},
			},
		},
		{
			name: "forward action - advanced schema",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.forward-multiple-tg": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"service-1","servicePort":"http","weight":20},{"serviceName":"service-2","servicePort":80,"weight":20},{"targetGroupARN":"tg-arn","weight":60}],"targetGroupStickinessConfig":{"enabled":true,"durationSeconds":200}}}`,
				},
				svcName: "forward-multiple-tg",
			},
			want: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							ServiceName: awssdk.String("service-1"),
							ServicePort: &portHTTP,
							Weight:      awssdk.Int64(20),
						},
						{
							ServiceName: awssdk.String("service-2"),
							ServicePort: &port80,
							Weight:      awssdk.Int64(20),
						},
						{
							TargetGroupARN: awssdk.String("tg-arn"),
							Weight:         awssdk.Int64(60),
						},
					},
					TargetGroupStickinessConfig: &TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int64(200),
					},
				},
			},
		},
		{
			name: "forward action - advanced schema - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.forward-multiple-tg": `{"Type":"forward","ForwardConfig":{"TargetGroups":[{"ServiceName":"service-1","ServicePort":"http","Weight":20},{"ServiceName":"service-2","ServicePort":"80","Weight":20},{"TargetGroupArn":"tg-arn","Weight":60}],"TargetGroupStickinessConfig":{"Enabled":true,"DurationSeconds":200}}}`,
				},
				svcName: "forward-multiple-tg",
			},
			want: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							ServiceName: awssdk.String("service-1"),
							ServicePort: &portHTTP,
							Weight:      awssdk.Int64(20),
						},
						{
							ServiceName: awssdk.String("service-2"),
							ServicePort: &port80,
							Weight:      awssdk.Int64(20),
						},
						{
							TargetGroupARN: awssdk.String("tg-arn"),
							Weight:         awssdk.Int64(60),
						},
					},
					TargetGroupStickinessConfig: &TargetGroupStickinessConfig{
						Enabled:         awssdk.Bool(true),
						DurationSeconds: awssdk.Int64(200),
					},
				},
			},
		},
		{
			name: "forward action - advanced schema - both string format and integer format integers should be supported",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.forward-multiple-tg": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"service-1","servicePort":"80","weight":40},{"serviceName":"service-2","servicePort":443,"weight":60}]}}`,
				},
				svcName: "forward-multiple-tg",
			},
			want: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							ServiceName: awssdk.String("service-1"),
							ServicePort: &port80,
							Weight:      awssdk.Int64(40),
						},
						{
							ServiceName: awssdk.String("service-2"),
							ServicePort: &port443,
							Weight:      awssdk.Int64(60),
						},
					},
				},
			},
		},
		{
			name: "redirect action",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.redirect-to-eks": `{"type":"redirect","redirectConfig":{"host":"aws.amazon.com","path":"/eks/","port":"443","protocol":"HTTPS","query":"k=v","statusCode":"HTTP_302"}}`,
				},
				svcName: "redirect-to-eks",
			},
			want: Action{
				Type: ActionTypeRedirect,
				RedirectConfig: &RedirectActionConfig{
					Host:       awssdk.String("aws.amazon.com"),
					Path:       awssdk.String("/eks/"),
					Port:       awssdk.String("443"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("k=v"),
					StatusCode: "HTTP_302",
				},
			},
		},
		{
			name: "redirect action - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.redirect-to-eks": `{"Type":"redirect","RedirectConfig":{"Host":"aws.amazon.com","Path":"/eks/","Port":"443","Protocol":"HTTPS","Query":"k=v","StatusCode":"HTTP_302"}}`,
				},
				svcName: "redirect-to-eks",
			},
			want: Action{
				Type: ActionTypeRedirect,
				RedirectConfig: &RedirectActionConfig{
					Host:       awssdk.String("aws.amazon.com"),
					Path:       awssdk.String("/eks/"),
					Port:       awssdk.String("443"),
					Protocol:   awssdk.String("HTTPS"),
					Query:      awssdk.String("k=v"),
					StatusCode: "HTTP_302",
				},
			},
		},
		{
			name: "redirect action - ssl-redirect",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.ssl-redirect": `{"type":"redirect","redirectConfig":{"port":"443","protocol":"HTTPS","statusCode":"HTTP_301"}}`,
				},
				svcName: "ssl-redirect",
			},
			want: Action{
				Type: ActionTypeRedirect,
				RedirectConfig: &RedirectActionConfig{
					Port:       awssdk.String("443"),
					Protocol:   awssdk.String("HTTPS"),
					StatusCode: "HTTP_301",
				},
			},
		},
		{
			name: "fixed response action",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.response-503": `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"503","messageBody":"503 error text"}}`,
				},
				svcName: "response-503",
			},
			want: Action{
				Type: ActionTypeFixedResponse,
				FixedResponseConfig: &FixedResponseActionConfig{
					ContentType: awssdk.String("text/plain"),
					MessageBody: awssdk.String("503 error text"),
					StatusCode:  "503",
				},
			},
		},
		{
			name: "fixed response action - old camelcase case json key",
			args: args{
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/actions.response-503": `{"Type":"fixed-response","FixedResponseConfig":{"ContentType":"text/plain","StatusCode":"503","MessageBody":"503 error text"}}`,
				},
				svcName: "response-503",
			},
			want: Action{
				Type: ActionTypeFixedResponse,
				FixedResponseConfig: &FixedResponseActionConfig{
					ContentType: awssdk.String("text/plain"),
					MessageBody: awssdk.String("503 error text"),
					StatusCode:  "503",
				},
			},
		},
		{
			name: "non-exists action",
			args: args{
				ingAnnotation: map[string]string{},
				svcName:       "non-exists",
			},
			wantErr: errors.New("missing actions.non-exists configuration"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			b := &defaultEnhancedBackendBuilder{
				annotationParser: annotationParser,
			}
			got, err := b.buildActionViaAnnotation(context.Background(), tt.args.ingAnnotation, tt.args.svcName)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got, "diff", cmp.Diff(tt.want, got))
			}
		})
	}
}

func Test_defaultEnhancedBackendBuilder_buildActionViaServiceAndServicePort(t *testing.T) {
	portHTTP := intstr.FromString("http")
	type args struct {
		svcName string
		svcPort intstr.IntOrString
	}
	tests := []struct {
		name string
		args args
		want Action
	}{
		{
			name: "standard case",
			args: args{
				svcName: "my-svc",
				svcPort: portHTTP,
			},
			want: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							ServiceName: awssdk.String("my-svc"),
							ServicePort: &portHTTP,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &defaultEnhancedBackendBuilder{}
			got := b.buildActionViaServiceAndServicePort(context.Background(), tt.args.svcName, tt.args.svcPort)

			assert.Equal(t, tt.want, got)
		})
	}
}
