package ingress

import (
	"context"
	"sort"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
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
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_defaultModelBuildTask_buildForwardAction(t *testing.T) {
	ing := ClassifiedIngress{
		Ing: &networking.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "awesome-ns",
				Name:      "ing-1",
				Annotations: map[string]string{
					"alb.ingress.kubernetes.io/target-type": "ip",
				},
			},
		},
	}
	portHTTP := intstr.FromString("http")
	svcSpec := corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			{
				Name: "http",
				Port: 80,
			},
		},
	}
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "awesome-ns",
			Name:        "svc-1",
			Annotations: map[string]string{},
		},
		Spec: svcSpec,
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "awesome-ns",
			Name:        "svc-2",
			Annotations: map[string]string{},
		},
		Spec: svcSpec,
	}
	svc3 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "awesome-ns-2",
			Name:        "svc-3",
			Annotations: map[string]string{},
		},
		Spec: svcSpec,
	}

	type args struct {
		ing       ClassifiedIngress
		actionCfg Action
	}
	type env struct {
		services []*corev1.Service
	}
	type want struct {
		targetGroupARN core.StringToken
		service        *corev1.Service
		weight         *int32
	}

	tests := []struct {
		name    string
		env     env
		args    args
		wants   []want
		wantErr error
	}{
		{
			name: "missing ForwardConfig",
			args: args{
				actionCfg: Action{},
			},
			wantErr: errors.New("missing ForwardConfig"),
		},
		{
			name: "single TargetGroup with TargetGroupARN",
			args: args{
				actionCfg: Action{
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								TargetGroupARN: awssdk.String("arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/my-target-group"),
							},
						},
					},
				},
			},
			wants: []want{
				{targetGroupARN: core.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/my-target-group")},
			},
		},
		{
			name: "single TargetGroup with service",
			env: env{
				services: []*corev1.Service{svc1},
			},
			args: args{
				ing: ing,
				actionCfg: Action{
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
			},
			wants: []want{
				{service: svc1},
			},
		},
		{
			name: "multiple TargetGroups with services",
			env: env{
				services: []*corev1.Service{svc1, svc2},
			},
			args: args{
				ing: ing,
				actionCfg: Action{
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
								Weight:      awssdk.Int32(80),
							},
							{
								ServiceName: awssdk.String("svc-2"),
								ServicePort: &portHTTP,
								Weight:      awssdk.Int32(20),
							},
						},
					},
				},
			},
			wants: []want{
				{service: svc1, weight: awssdk.Int32(80)},
				{service: svc2, weight: awssdk.Int32(20)},
			},
		},
		{
			name: "multiple TargetGroups with mix of TargetGroupARN and service",
			env: env{
				services: []*corev1.Service{svc1},
			},
			args: args{
				ing: ing,
				actionCfg: Action{
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								TargetGroupARN: awssdk.String("arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/my-target-group"),
								Weight:         awssdk.Int32(80),
							},
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
								Weight:      awssdk.Int32(20),
							},
						},
					},
				},
			},
			wants: []want{
				{
					targetGroupARN: core.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/my-target-group"),
					weight:         awssdk.Int32((80)),
				},
				{
					service: svc1,
					weight:  awssdk.Int32(20),
				},
			},
		},
		{
			name: "multiple TargetGroups without weight",
			env: env{
				services: []*corev1.Service{svc1, svc2},
			},
			args: args{
				ing: ing,
				actionCfg: Action{
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
							},
							{
								ServiceName: awssdk.String("svc-2"),
								ServicePort: &portHTTP,
							},
						},
					},
				},
			},
			wants: []want{
				{service: svc1},
				{service: svc2},
			},
		},
		{
			name: "multiple TargetGroups with services in separate namespaces",
			env: env{
				services: []*corev1.Service{svc1, svc3},
			},
			args: args{
				ing: ing,
				actionCfg: Action{
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portHTTP,
								Weight:      awssdk.Int32(80),
							},
							{
								ServiceNamespace: awssdk.String("awesome-ns-2"),
								ServiceName:      awssdk.String("svc-3"),
								ServicePort:      &portHTTP,
								Weight:           awssdk.Int32(20),
							},
						},
					},
				},
			},
			wants: []want{
				{service: svc1, weight: awssdk.Int32(80)},
				{service: svc3, weight: awssdk.Int32(20)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			backendServices := map[types.NamespacedName]*corev1.Service{}
			for _, service := range tt.env.services {
				err := k8sClient.Create(context.Background(), service.DeepCopy())
				assert.NoError(t, err)
				key := types.NamespacedName{Namespace: service.ObjectMeta.Namespace, Name: service.ObjectMeta.Name}
				backendServices[key] = service
			}
			stack := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "awesome-ns", Name: "ing-1"}))
			task := &defaultModelBuildTask{
				k8sClient:                     k8sClient,
				annotationParser:              annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				backendServices:               backendServices,
				enableIPTargetType:            true,
				defaultBackendProtocol:        elbv2model.ProtocolHTTP,
				defaultBackendProtocolVersion: elbv2model.ProtocolVersionHTTP1,
				stack:                         stack,
				tgByResID:                     make(map[string]*elbv2model.TargetGroup),
			}
			got, err := task.buildForwardAction(context.Background(), tt.args.ing, tt.args.actionCfg)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, elbv2model.ActionTypeForward, got.Type)
				assert.NotNil(t, got.ForwardConfig)

				var gotTargetGroupBindings []*elbv2model.TargetGroupBindingResource
				err = stack.ListResources(&gotTargetGroupBindings)
				assert.NoError(t, err)
				sort.Slice(gotTargetGroupBindings, func(i, j int) bool {
					return gotTargetGroupBindings[i].ID() < gotTargetGroupBindings[j].ID()
				})

				tgbIndex := 0
				for i, want := range tt.wants {
					if want.targetGroupARN != nil {
						assert.Equal(t, want.targetGroupARN, got.ForwardConfig.TargetGroups[i].TargetGroupARN)
					}
					if want.service != nil {
						tgb := gotTargetGroupBindings[tgbIndex]
						assert.Equal(t, want.service.ObjectMeta.Namespace, tgb.Spec.Template.ObjectMeta.Namespace)
						assert.Equal(t, want.service.ObjectMeta.Name, tgb.Spec.Template.Spec.ServiceRef.Name)
						assert.Equal(t, intstr.FromString(want.service.Spec.Ports[0].Name), tgb.Spec.Template.Spec.ServiceRef.Port)
						assert.Equal(t, want.weight, got.ForwardConfig.TargetGroups[i].Weight)
						tgbIndex += 1
					}
				}
			}
		})
	}
}

func Test_defaultModelBuildTask_buildAuthenticateOIDCAction(t *testing.T) {
	type env struct {
		secrets []*corev1.Secret
	}
	type args struct {
		authCfg   AuthConfig
		namespace string
	}
	authBehaviorAuthenticate := elbv2model.AuthenticateOIDCActionConditionalBehaviorAuthenticate
	tests := []struct {
		name    string
		env     env
		args    args
		want    elbv2model.Action
		wantErr error
	}{
		{
			name: "clientID & clientSecret configured",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientID":     []byte("my-client-id"),
							"clientSecret": []byte("my-client-secret"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			want: elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateOIDC,
				AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{
					Issuer:                "https://example.com",
					AuthorizationEndpoint: "https://authorization.example.com",
					TokenEndpoint:         "https://token.example.com",
					UserInfoEndpoint:      "https://userinfo.example.co",
					ClientID:              "my-client-id",
					ClientSecret:          "my-client-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key1": "value1",
					},
					OnUnauthenticatedRequest: authBehaviorAuthenticate,
					Scope:                    awssdk.String("email"),
					SessionCookieName:        awssdk.String("my-session-cookie"),
					SessionTimeout:           awssdk.Int64(65536),
				},
			},
		},
		{
			name: "clientID & clientSecret configured - legacy clientId",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientId":     []byte("my-client-id"),
							"clientSecret": []byte("my-client-secret"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			want: elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateOIDC,
				AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{
					Issuer:                "https://example.com",
					AuthorizationEndpoint: "https://authorization.example.com",
					TokenEndpoint:         "https://token.example.com",
					UserInfoEndpoint:      "https://userinfo.example.co",
					ClientID:              "my-client-id",
					ClientSecret:          "my-client-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key1": "value1",
					},
					OnUnauthenticatedRequest: authBehaviorAuthenticate,
					Scope:                    awssdk.String("email"),
					SessionCookieName:        awssdk.String("my-session-cookie"),
					SessionTimeout:           awssdk.Int64(65536),
				},
			},
		},
		{
			name: "missing IDPConfigOIDC",
			args: args{
				authCfg: AuthConfig{
					Type:          AuthTypeCognito,
					IDPConfigOIDC: nil,
				},
			},
			wantErr: errors.New("missing IDPConfigOIDC"),
		},
		{
			name: "missing clientID",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientSecret": []byte("my-client-secret"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			wantErr: errors.New("missing clientID, secret: my-ns/my-k8s-secret"),
		},
		{
			name: "missing clientSecret",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientID": []byte("my-client-id"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			wantErr: errors.New("missing clientSecret, secret: my-ns/my-k8s-secret"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, secret := range tt.env.secrets {
				err := k8sClient.Create(context.Background(), secret.DeepCopy())
				assert.NoError(t, err)
			}

			task := &defaultModelBuildTask{
				k8sClient: k8sClient,
			}
			got, err := task.buildAuthenticateOIDCAction(context.Background(), tt.args.namespace, tt.args.authCfg)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildSSLRedirectAction(t *testing.T) {
	type args struct {
		sslRedirectConfig SSLRedirectConfig
	}
	tests := []struct {
		name string
		args args
		want elbv2model.Action
	}{
		{
			name: "SSLRedirect to 443 with 301",
			args: args{
				sslRedirectConfig: SSLRedirectConfig{
					SSLPort:    443,
					StatusCode: "HTTP_301",
				},
			},
			want: elbv2model.Action{
				Type: elbv2model.ActionTypeRedirect,
				RedirectConfig: &elbv2model.RedirectActionConfig{
					Port:       awssdk.String("443"),
					Protocol:   awssdk.String("HTTPS"),
					StatusCode: "HTTP_301",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.buildSSLRedirectAction(context.Background(), tt.args.sslRedirectConfig)
			assert.Equal(t, tt.want, got)
		})
	}
}
