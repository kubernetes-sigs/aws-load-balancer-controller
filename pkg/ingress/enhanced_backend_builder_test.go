package ingress

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_defaultEnhancedBackendBuilder_Build(t *testing.T) {
	type env struct {
		svcs []*corev1.Service
	}
	type fields struct {
		tolerateNonExistentBackendService bool
		tolerateNonExistentBackendAction  bool
	}
	type args struct {
		ing     *networking.Ingress
		backend networking.IngressBackend

		loadBackendServices bool
		loadAuthConfig      bool
		backendServices     map[types.NamespacedName]*corev1.Service
	}

	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "svc-1",
		},
	}
	portHTTP := intstr.FromString("http")
	backendPortHTTP := networking.ServiceBackendPort{Name: "http"}
	tests := []struct {
		name                string
		env                 env
		fields              fields
		args                args
		want                EnhancedBackend
		wantBackendServices map[types.NamespacedName]*corev1.Service
		wantErr             error
	}{
		{
			name: "vanilla serviceBackend",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "awesome-ns",
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "svc-1",
						Port: backendPortHTTP,
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				AuthConfig: AuthConfig{
					Type:                     AuthTypeNone,
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "vanilla serviceBackend with additional conditions",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/conditions.svc-1": `[{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue1", "HeaderValue2"]}}]`,
						},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "svc-1",
						Port: backendPortHTTP,
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldHTTPHeader,
						HTTPHeaderConfig: &HTTPHeaderConditionConfig{
							HTTPHeaderName: "HeaderName",
							Values:         []string{"HeaderValue1", "HeaderValue2"},
						},
					},
				},
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				AuthConfig: AuthConfig{
					Type:                     AuthTypeNone,
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "vanilla serviceBackend with additional auth configuration",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/auth-type":        "cognito",
							"alb.ingress.kubernetes.io/auth-idp-cognito": "{\"userPoolARN\":\"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx\",\"userPoolClientID\":\"my-clientID\",\"userPoolDomain\":\"my-domain\"}",
						},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "svc-1",
						Port: backendPortHTTP,
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				AuthConfig: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigCognito: &AuthIDPConfigCognito{
						UserPoolARN:      "arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx",
						UserPoolClientID: "my-clientID",
						UserPoolDomain:   "my-domain",
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "vanilla serviceBackend - non-existent service and tolerateNonExistentBackendService==true",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "awesome-ns",
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "svc-2",
						Port: backendPortHTTP,
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeFixedResponse,
					FixedResponseConfig: &FixedResponseActionConfig{
						ContentType: awssdk.String("text/plain"),
						StatusCode:  "503",
						MessageBody: awssdk.String(nonExistentBackendServiceMessageBody),
					},
				},
				AuthConfig: AuthConfig{
					Type:                     AuthTypeNone,
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{},
		},
		{
			name: "vanilla serviceBackend - non-existent service and tolerateNonExistentBackendService==false",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: false,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "awesome-ns",
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "svc-2",
						Port: backendPortHTTP,
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			wantErr: errors.New("services \"svc-2\" not found"),
		},
		{
			name: "vanilla serviceBackend - non-existent service and tolerateNonExistentBackendService==false without loadBackendServices ",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: false,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "awesome-ns",
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "svc-2",
						Port: backendPortHTTP,
					},
				},
				loadBackendServices: false,
				loadAuthConfig:      false,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-2"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{},
		},
		{
			name: "annotation-based serviceBackend",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/actions.fake-my-svc": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc-1","servicePort":"http"}]}}`,
						},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "fake-my-svc",
						Port: networking.ServiceBackendPort{
							Name: "use-annotation",
						},
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				AuthConfig: AuthConfig{
					Type:                     AuthTypeNone,
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "annotation-based with additional conditions",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/actions.fake-my-svc":    `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc-1","servicePort":"http"}]}}`,
							"alb.ingress.kubernetes.io/conditions.fake-my-svc": `[{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue1", "HeaderValue2"]}}]`,
						},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "fake-my-svc",
						Port: networking.ServiceBackendPort{
							Name: "use-annotation",
						},
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldHTTPHeader,
						HTTPHeaderConfig: &HTTPHeaderConditionConfig{
							HTTPHeaderName: "HeaderName",
							Values:         []string{"HeaderValue1", "HeaderValue2"},
						},
					},
				},
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				AuthConfig: AuthConfig{
					Type:                     AuthTypeNone,
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "annotation-based with additional auth configuration",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/actions.fake-my-svc": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc-1","servicePort":"http"}]}}`,
							"alb.ingress.kubernetes.io/auth-type":           "cognito",
							"alb.ingress.kubernetes.io/auth-idp-cognito":    "{\"userPoolARN\":\"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx\",\"userPoolClientID\":\"my-clientID\",\"userPoolDomain\":\"my-domain\"}",
						},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "fake-my-svc",
						Port: networking.ServiceBackendPort{
							Name: "use-annotation",
						},
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
				AuthConfig: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigCognito: &AuthIDPConfigCognito{
						UserPoolARN:      "arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx",
						UserPoolClientID: "my-clientID",
						UserPoolDomain:   "my-domain",
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "annotation-based serviceBackend - non-existent action and tolerateNonExistentBackendAction==true",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "awesome-ns",
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "fake-my-svc",
						Port: networking.ServiceBackendPort{
							Name: "use-annotation",
						},
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			want: EnhancedBackend{
				Action: Action{
					Type: ActionTypeFixedResponse,
					FixedResponseConfig: &FixedResponseActionConfig{
						ContentType: awssdk.String("text/plain"),
						StatusCode:  "503",
						MessageBody: awssdk.String(nonExistentBackendActionMessageBody),
					},
				},
				AuthConfig: AuthConfig{
					Type:                     AuthTypeNone,
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "openid",
					SessionCookieName:        "AWSELBAuthSessionCookie",
					SessionTimeout:           604800,
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{},
		},
		{
			name: "annotation-based serviceBackend - non-existent action and tolerateNonExistentBackendAction==false",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "awesome-ns",
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: "fake-my-svc",
						Port: networking.ServiceBackendPort{
							Name: "use-annotation",
						},
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			wantErr: errors.New("missing actions.fake-my-svc configuration"),
		},
		{
			name: "resource backend",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
				tolerateNonExistentBackendAction:  true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "awesome-ns",
						Annotations: map[string]string{},
					},
				},
				backend: networking.IngressBackend{
					Resource: &corev1.TypedLocalObjectReference{
						APIGroup: awssdk.String("v1"),
						Kind:     "Service",
						Name:     "awesome-service",
					},
				},
				loadBackendServices: true,
				loadAuthConfig:      true,
				backendServices:     map[types.NamespacedName]*corev1.Service{},
			},
			wantErr: errors.New("missing required \"service\" field"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, svc := range tt.env.svcs {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			authConfigBuilder := NewDefaultAuthConfigBuilder(annotationParser)
			b := &defaultEnhancedBackendBuilder{
				k8sClient:                         k8sClient,
				annotationParser:                  annotationParser,
				authConfigBuilder:                 authConfigBuilder,
				tolerateNonExistentBackendService: tt.fields.tolerateNonExistentBackendService,
				tolerateNonExistentBackendAction:  tt.fields.tolerateNonExistentBackendAction,
			}

			got, err := b.Build(context.Background(), tt.args.ing, tt.args.backend,
				WithLoadBackendServices(tt.args.loadBackendServices, tt.args.backendServices),
				WithLoadAuthConfig(tt.args.loadAuthConfig))
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got, "diff", cmp.Diff(tt.want, got))
				if tt.args.loadBackendServices {
					opt := equality.IgnoreFakeClientPopulatedFields()
					assert.True(t, cmp.Equal(tt.wantBackendServices, tt.args.backendServices, opt),
						"diff: %v", cmp.Diff(tt.wantBackendServices, tt.args.backendServices, opt))
				}
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

func Test_defaultEnhancedBackendBuilder_loadBackendServices(t *testing.T) {
	port80 := intstr.FromInt(80)
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "svc-1",
			Annotations: map[string]string{
				"version": "2",
			},
		},
	}
	svc1_v1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "svc-1",
			Annotations: map[string]string{
				"version": "1",
			},
		},
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "svc-2",
		},
	}

	type env struct {
		svcs []*corev1.Service
	}
	type fields struct {
		tolerateNonExistentBackendService bool
	}
	type args struct {
		action          *Action
		namespace       string
		backendServices map[types.NamespacedName]*corev1.Service
	}
	tests := []struct {
		name                string
		env                 env
		fields              fields
		args                args
		wantAction          Action
		wantBackendServices map[types.NamespacedName]*corev1.Service
		wantErr             error
	}{
		{
			name: "forward to a single exists service",
			env: env{
				svcs: []*corev1.Service{svc1, svc2},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantAction: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							ServiceName: awssdk.String("svc-1"),
							ServicePort: &port80,
						},
					},
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "forward to multiple exists service",
			env: env{
				svcs: []*corev1.Service{svc1, svc2},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
							{
								ServiceName: awssdk.String("svc-2"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantAction: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							ServiceName: awssdk.String("svc-1"),
							ServicePort: &port80,
						},
						{
							ServiceName: awssdk.String("svc-2"),
							ServicePort: &port80,
						},
					},
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-2"}: svc2,
			},
		},
		{
			name: "forward to multiple exists service - svc1 has older snapshot",
			env: env{
				svcs: []*corev1.Service{svc1, svc2},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
							{
								ServiceName: awssdk.String("svc-2"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace: "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{
					types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1_v1,
				},
			},
			wantAction: Action{
				Type: ActionTypeForward,
				ForwardConfig: &ForwardActionConfig{
					TargetGroups: []TargetGroupTuple{
						{
							ServiceName: awssdk.String("svc-1"),
							ServicePort: &port80,
						},
						{
							ServiceName: awssdk.String("svc-2"),
							ServicePort: &port80,
						},
					},
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1_v1,
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-2"}: svc2,
			},
		},
		{
			name: "forward to a single non-existent service - tolerateNonExistentBackendService == true",
			env: env{
				svcs: []*corev1.Service{},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantAction: Action{
				Type: ActionTypeFixedResponse,
				FixedResponseConfig: &FixedResponseActionConfig{
					ContentType: awssdk.String("text/plain"),
					StatusCode:  "503",
					MessageBody: awssdk.String(nonExistentBackendServiceMessageBody),
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{},
		},
		{
			name: "forward to a single non-existent service - tolerateNonExistentBackendService == false",
			env: env{
				svcs: []*corev1.Service{},
			},
			fields: fields{
				tolerateNonExistentBackendService: false,
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantErr: errors.New("services \"svc-1\" not found"),
		},
		{
			name: "forward to multiple services, one of them is non-existent - tolerateNonExistentBackendService == true",
			env: env{
				svcs: []*corev1.Service{svc1},
			},
			fields: fields{
				tolerateNonExistentBackendService: true,
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
							{
								ServiceName: awssdk.String("svc-2"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantErr: errors.New("services \"svc-2\" not found"),
		},
		{
			name: "load for fixed response action is noop",
			fields: fields{
				tolerateNonExistentBackendService: true,
			},
			args: args{
				action: &Action{
					Type: ActionTypeFixedResponse,
					FixedResponseConfig: &FixedResponseActionConfig{
						ContentType: awssdk.String("text/plain"),
						StatusCode:  "404",
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantAction: Action{
				Type: ActionTypeFixedResponse,
				FixedResponseConfig: &FixedResponseActionConfig{
					ContentType: awssdk.String("text/plain"),
					StatusCode:  "404",
				},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, svc := range tt.env.svcs {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}

			b := &defaultEnhancedBackendBuilder{
				k8sClient:                         k8sClient,
				tolerateNonExistentBackendService: tt.fields.tolerateNonExistentBackendService,
			}
			err := b.loadBackendServices(ctx, tt.args.action, tt.args.namespace, tt.args.backendServices)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantAction, *tt.args.action)
				opt := equality.IgnoreFakeClientPopulatedFields()
				assert.True(t, cmp.Equal(tt.wantBackendServices, tt.args.backendServices, opt),
					"diff: %v", cmp.Diff(tt.wantBackendServices, tt.args.backendServices, opt))
			}
		})
	}
}

func Test_defaultEnhancedBackendBuilder_buildAuthConfig(t *testing.T) {
	port80 := intstr.FromInt(80)
	type args struct {
		action          Action
		namespace       string
		ingAnnotation   map[string]string
		backendServices map[types.NamespacedName]*corev1.Service
	}
	tests := []struct {
		name    string
		args    args
		want    AuthConfig
		wantErr error
	}{
		{
			name: "forward action with single targetGroup will load authConfig from Ingress & Service",
			args: args{
				action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("my-svc"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace: "awesome-ns",
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/auth-type":        "cognito",
					"alb.ingress.kubernetes.io/auth-idp-cognito": "{\"userPoolARN\":\"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx\",\"userPoolClientID\":\"my-clientID\",\"userPoolDomain\":\"my-domain\"}",
				},
				backendServices: map[types.NamespacedName]*corev1.Service{
					types.NamespacedName{Namespace: "awesome-ns", Name: "my-svc"}: {
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/auth-type": "none",
							},
						},
					},
				},
			},
			want: AuthConfig{
				Type: AuthTypeNone,
				IDPConfigCognito: &AuthIDPConfigCognito{
					UserPoolARN:      "arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx",
					UserPoolClientID: "my-clientID",
					UserPoolDomain:   "my-domain",
				},
				OnUnauthenticatedRequest: "authenticate",
				Scope:                    "openid",
				SessionCookieName:        "AWSELBAuthSessionCookie",
				SessionTimeout:           604800,
			},
		},
		{
			name: "forward action with multiple targetGroup will load authConfig from Ingress",
			args: args{
				action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								TargetGroupARN: awssdk.String("my-tg-arn"),
							},
							{
								ServiceName: awssdk.String("my-svc"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace: "awesome-ns",
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/auth-type":        "cognito",
					"alb.ingress.kubernetes.io/auth-idp-cognito": "{\"userPoolARN\":\"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx\",\"userPoolClientID\":\"my-clientID\",\"userPoolDomain\":\"my-domain\"}",
				},
				backendServices: map[types.NamespacedName]*corev1.Service{
					types.NamespacedName{Namespace: "awesome-ns", Name: "my-svc"}: {
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/auth-type": "none",
							},
						},
					},
				},
			},
			want: AuthConfig{
				Type: AuthTypeCognito,
				IDPConfigCognito: &AuthIDPConfigCognito{
					UserPoolARN:      "arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx",
					UserPoolClientID: "my-clientID",
					UserPoolDomain:   "my-domain",
				},
				OnUnauthenticatedRequest: "authenticate",
				Scope:                    "openid",
				SessionCookieName:        "AWSELBAuthSessionCookie",
				SessionTimeout:           604800,
			},
		},
		{
			name: "fixed response action will load authConfig from Ingress",
			args: args{
				action: Action{
					Type: ActionTypeFixedResponse,
					FixedResponseConfig: &FixedResponseActionConfig{
						ContentType: awssdk.String("text/plain"),
						StatusCode:  "404",
					},
				},
				namespace: "awesome-ns",
				ingAnnotation: map[string]string{
					"alb.ingress.kubernetes.io/auth-type":        "cognito",
					"alb.ingress.kubernetes.io/auth-idp-cognito": "{\"userPoolARN\":\"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx\",\"userPoolClientID\":\"my-clientID\",\"userPoolDomain\":\"my-domain\"}",
				},
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			want: AuthConfig{
				Type: AuthTypeCognito,
				IDPConfigCognito: &AuthIDPConfigCognito{
					UserPoolARN:      "arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx",
					UserPoolClientID: "my-clientID",
					UserPoolDomain:   "my-domain",
				},
				OnUnauthenticatedRequest: "authenticate",
				Scope:                    "openid",
				SessionCookieName:        "AWSELBAuthSessionCookie",
				SessionTimeout:           604800,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			authConfigBuilder := NewDefaultAuthConfigBuilder(annotationParser)
			b := &defaultEnhancedBackendBuilder{
				annotationParser:  annotationParser,
				authConfigBuilder: authConfigBuilder,
			}
			got, err := b.buildAuthConfig(context.Background(), tt.args.action, tt.args.namespace, tt.args.ingAnnotation, tt.args.backendServices)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultEnhancedBackendBuilder_build503ResponseAction(t *testing.T) {
	type args struct {
		messageBody string
	}
	tests := []struct {
		name string
		args args
		want Action
	}{
		{
			name: "non-existent service",
			args: args{
				messageBody: "Backend service does not exist",
			},
			want: Action{
				Type: ActionTypeFixedResponse,
				FixedResponseConfig: &FixedResponseActionConfig{
					ContentType: awssdk.String("text/plain"),
					StatusCode:  "503",
					MessageBody: awssdk.String("Backend service does not exist"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &defaultEnhancedBackendBuilder{}
			got := b.build503ResponseAction(tt.args.messageBody)
			assert.Equal(t, tt.want, got)
		})
	}
}
