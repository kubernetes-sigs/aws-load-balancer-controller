package auth

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	mock_cache "github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks/controller-runtime/cache"
	mock_controller "github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks/controller-runtime/controller"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func TestDefaultModule_Init(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ingressChan := make(chan event.GenericEvent)
	serviceChan := make(chan event.GenericEvent)
	mockCache := mock_cache.NewMockCache(ctrl)
	mockCache.EXPECT().IndexField(&extensions.Ingress{}, FieldAuthOIDCSecret, gomock.Any())
	mockCache.EXPECT().IndexField(&corev1.Service{}, FieldAuthOIDCSecret, gomock.Any())
	mockController := mock_controller.NewMockController(ctrl)
	mockController.EXPECT().Watch(&source.Kind{Type: &corev1.Secret{}}, &EnqueueRequestsForSecretEvent{
		IngressChan: ingressChan,
		ServiceChan: serviceChan,
		Cache:       mockCache,
	})

	module := &defaultModule{mockCache}
	assert.NoError(t, module.Init(mockController, ingressChan, serviceChan))
}

func TestBuildOIDCSecretIndex(t *testing.T) {
	for _, tc := range []struct {
		name            string
		namespace       string
		annotations     map[string]string
		expectedIndexes []string
	}{
		{
			name:            "ingress/service don't use OIDC auth",
			namespace:       "namespace",
			annotations:     nil,
			expectedIndexes: nil,
		},
		{
			name:      "ingress/service use OIDC auth",
			namespace: "namespace",
			annotations: map[string]string{
				parser.GetAnnotationWithPrefix(AnnotationAuthIDPOIDC): "{\"SecretName\": \"oidc-secret\"}",
			},
			expectedIndexes: []string{"namespace/oidc-secret"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actualIndexes := buildOIDCSecretIndex(tc.namespace, tc.annotations)
			assert.Equal(t, actualIndexes, tc.expectedIndexes)
		})
	}
}

func TestDefaultModule_NewConfig(t *testing.T) {
	for _, tc := range []struct {
		name            string
		ingress         *extensions.Ingress
		backend         extensions.IngressBackend
		service         *corev1.Service
		secret          *corev1.Secret
		protocol        string
		expectedAuthCfg Config
		expectedErr     error
	}{
		{
			name: "ingress use cognito auth",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):       "cognito",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPCognito): "{\"UserPoolArn\": \"UserPoolArn\",\"UserPoolClientId\": \"UserPoolClientId\",\"UserPoolDomain\": \"UserPoolDomain\",\"AuthenticationRequestExtraParams\": { \"param1\": \"value1\",\"param2\": \"value2\"}}",
					},
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "service",
				},
			},
			protocol: "HTTPS",
			expectedAuthCfg: Config{
				Type: TypeCognito,
				IDPCognito: IDPCognito{
					AuthenticationRequestExtraParams: AuthenticationRequestExtraParams{
						"param1": "value1",
						"param2": "value2",
					},
					UserPoolArn:      "UserPoolArn",
					UserPoolClientId: "UserPoolClientId",
					UserPoolDomain:   "UserPoolDomain",
				},
				Scope:                    DefaultAuthScope,
				SessionCookie:            DefaultAuthSessionCookie,
				SessionTimeout:           DefaultAuthSessionTimeout,
				OnUnauthenticatedRequest: DefaultAuthOnUnauthenticatedRequest,
			},
		},
		{
			name: "service use cognito auth",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "service",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):       "cognito",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPCognito): "{\"UserPoolArn\": \"UserPoolArn\",\"UserPoolClientId\": \"UserPoolClientId\",\"UserPoolDomain\": \"UserPoolDomain\",\"AuthenticationRequestExtraParams\": { \"param1\": \"value1\",\"param2\": \"value2\"}}",
					},
				},
			},
			protocol: "HTTPS",
			expectedAuthCfg: Config{
				Type: TypeCognito,
				IDPCognito: IDPCognito{
					AuthenticationRequestExtraParams: AuthenticationRequestExtraParams{
						"param1": "value1",
						"param2": "value2",
					},
					UserPoolArn:      "UserPoolArn",
					UserPoolClientId: "UserPoolClientId",
					UserPoolDomain:   "UserPoolDomain",
				},
				Scope:                    DefaultAuthScope,
				SessionCookie:            DefaultAuthSessionCookie,
				SessionTimeout:           DefaultAuthSessionTimeout,
				OnUnauthenticatedRequest: DefaultAuthOnUnauthenticatedRequest,
			},
		},
		{
			name: "ingress use oidc auth",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):    "oidc",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPOIDC): "{\"Issuer\": \"Issuer\",\"AuthorizationEndpoint\": \"AuthorizationEndpoint\",\"TokenEndpoint\": \"TokenEndpoint\",\"UserInfoEndpoint\": \"UserInfoEndpoint\",\"SecretName\": \"oidc-secret\",\"AuthenticationRequestExtraParams\": { \"param1\": \"value1\",\"param2\": \"value2\"}}",
					},
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "service",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "oidc-secret",
				},
				Data: map[string][]byte{
					"clientId":     []byte("clientId"),
					"clientSecret": []byte("clientSecret"),
				},
			},
			protocol: "HTTPS",
			expectedAuthCfg: Config{
				Type: TypeOIDC,
				IDPOIDC: IDPOIDC{
					Issuer:                "Issuer",
					AuthorizationEndpoint: "AuthorizationEndpoint",
					AuthenticationRequestExtraParams: AuthenticationRequestExtraParams{
						"param1": "value1",
						"param2": "value2",
					},
					TokenEndpoint:    "TokenEndpoint",
					UserInfoEndpoint: "UserInfoEndpoint",
					ClientId:         "clientId",
					ClientSecret:     "clientSecret",
				},
				Scope:                    DefaultAuthScope,
				SessionCookie:            DefaultAuthSessionCookie,
				SessionTimeout:           DefaultAuthSessionTimeout,
				OnUnauthenticatedRequest: DefaultAuthOnUnauthenticatedRequest,
			},
		},
		{
			name: "service use oidc auth clientId with trailing whitespaces",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "service",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):    "oidc",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPOIDC): "{\"Issuer\": \"Issuer\",\"AuthorizationEndpoint\": \"AuthorizationEndpoint\",\"TokenEndpoint\": \"TokenEndpoint\",\"UserInfoEndpoint\": \"UserInfoEndpoint\",\"SecretName\": \"oidc-secret\",\"AuthenticationRequestExtraParams\": { \"param1\": \"value1\",\"param2\": \"value2\"}}",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "oidc-secret",
				},
				Data: map[string][]byte{
					"clientId":     []byte("clientId\t \n"),
					"clientSecret": []byte("clientSecret"),
				},
			},
			protocol: "HTTPS",
			expectedAuthCfg: Config{
				Type: TypeOIDC,
				IDPOIDC: IDPOIDC{
					Issuer:                "Issuer",
					AuthorizationEndpoint: "AuthorizationEndpoint",
					AuthenticationRequestExtraParams: AuthenticationRequestExtraParams{
						"param1": "value1",
						"param2": "value2",
					},
					TokenEndpoint:    "TokenEndpoint",
					UserInfoEndpoint: "UserInfoEndpoint",
					ClientId:         "clientId",
					ClientSecret:     "clientSecret",
				},
				Scope:                    DefaultAuthScope,
				SessionCookie:            DefaultAuthSessionCookie,
				SessionTimeout:           DefaultAuthSessionTimeout,
				OnUnauthenticatedRequest: DefaultAuthOnUnauthenticatedRequest,
			},
		},
		{
			name: "service use oidc auth",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "service",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):    "oidc",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPOIDC): "{\"Issuer\": \"Issuer\",\"AuthorizationEndpoint\": \"AuthorizationEndpoint\",\"TokenEndpoint\": \"TokenEndpoint\",\"UserInfoEndpoint\": \"UserInfoEndpoint\",\"SecretName\": \"oidc-secret\",\"AuthenticationRequestExtraParams\": { \"param1\": \"value1\",\"param2\": \"value2\"}}",
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "oidc-secret",
				},
				Data: map[string][]byte{
					"clientId":     []byte("clientId"),
					"clientSecret": []byte("clientSecret"),
				},
			},
			protocol: "HTTPS",
			expectedAuthCfg: Config{
				Type: TypeOIDC,
				IDPOIDC: IDPOIDC{
					Issuer:                "Issuer",
					AuthorizationEndpoint: "AuthorizationEndpoint",
					AuthenticationRequestExtraParams: AuthenticationRequestExtraParams{
						"param1": "value1",
						"param2": "value2",
					},
					TokenEndpoint:    "TokenEndpoint",
					UserInfoEndpoint: "UserInfoEndpoint",
					ClientId:         "clientId",
					ClientSecret:     "clientSecret",
				},
				Scope:                    DefaultAuthScope,
				SessionCookie:            DefaultAuthSessionCookie,
				SessionTimeout:           DefaultAuthSessionTimeout,
				OnUnauthenticatedRequest: DefaultAuthOnUnauthenticatedRequest,
			},
		},
		{
			name: "ingress with custom auth settings",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):                     "cognito",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPCognito):               "{\"UserPoolArn\": \"UserPoolArn\",\"UserPoolClientId\": \"UserPoolClientId\",\"UserPoolDomain\": \"UserPoolDomain\"}",
						parser.GetAnnotationWithPrefix(AnnotationAuthScope):                    "email openid",
						parser.GetAnnotationWithPrefix(AnnotationAuthSessionCookie):            "customCookieName",
						parser.GetAnnotationWithPrefix(AnnotationAuthSessionTimeout):           "600",
						parser.GetAnnotationWithPrefix(AnnotationAuthOnUnauthenticatedRequest): "allow",
					},
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "service",
				},
			},
			protocol: "HTTPS",
			expectedAuthCfg: Config{
				Type: TypeCognito,
				IDPCognito: IDPCognito{
					UserPoolArn:      "UserPoolArn",
					UserPoolClientId: "UserPoolClientId",
					UserPoolDomain:   "UserPoolDomain",
				},
				Scope:                    "email openid",
				SessionCookie:            "customCookieName",
				SessionTimeout:           600,
				OnUnauthenticatedRequest: OnUnauthenticatedRequestAllow,
			},
		},
		{
			name: "service override auth settings on ingress",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):                     "cognito",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPCognito):               "{\"UserPoolArn\": \"UserPoolArn\",\"UserPoolClientId\": \"UserPoolClientId\",\"UserPoolDomain\": \"UserPoolDomain\"}",
						parser.GetAnnotationWithPrefix(AnnotationAuthScope):                    "email openid",
						parser.GetAnnotationWithPrefix(AnnotationAuthSessionCookie):            "customCookieName",
						parser.GetAnnotationWithPrefix(AnnotationAuthSessionTimeout):           "600",
						parser.GetAnnotationWithPrefix(AnnotationAuthOnUnauthenticatedRequest): "allow",
					},
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "service",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthOnUnauthenticatedRequest): "deny",
					},
				},
			},
			protocol: "HTTPS",
			expectedAuthCfg: Config{
				Type: TypeCognito,
				IDPCognito: IDPCognito{
					UserPoolArn:      "UserPoolArn",
					UserPoolClientId: "UserPoolClientId",
					UserPoolDomain:   "UserPoolDomain",
				},
				Scope:                    "email openid",
				SessionCookie:            "customCookieName",
				SessionTimeout:           600,
				OnUnauthenticatedRequest: OnUnauthenticatedRequestDeny,
			},
		},
		{
			name: "http don't support auth",
			ingress: &extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						parser.GetAnnotationWithPrefix(AnnotationAuthType):       "cognito",
						parser.GetAnnotationWithPrefix(AnnotationAuthIDPCognito): "{\"UserPoolArn\": \"UserPoolArn\",\"UserPoolClientId\": \"UserPoolClientId\",\"UserPoolDomain\": \"UserPoolDomain\"}",
					},
				},
			},
			backend: extensions.IngressBackend{
				ServiceName: "service",
				ServicePort: intstr.FromInt(80),
			},
			protocol: "HTTP",
			expectedAuthCfg: Config{
				Type: TypeNone,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCache := mock_cache.NewMockCache(ctrl)
			if tc.service != nil {
				mockCache.EXPECT().Get(gomock.Any(), types.NamespacedName{
					Namespace: tc.service.Namespace,
					Name:      tc.service.Name,
				}, gomock.Any()).SetArg(2, *tc.service)
			}
			if tc.secret != nil {
				mockCache.EXPECT().Get(gomock.Any(), types.NamespacedName{
					Namespace: tc.secret.Namespace,
					Name:      tc.secret.Name,
				}, gomock.Any()).SetArg(2, *tc.secret)
			}
			module := &defaultModule{mockCache}

			authCfg, err := module.NewConfig(context.Background(), tc.ingress, tc.backend, tc.protocol)
			assert.Equal(t, authCfg, tc.expectedAuthCfg)
			assert.Equal(t, err, tc.expectedErr)
		})
	}
}
