package auth

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	AnnotationAuthType                     string = "auth-type"
	AnnotationAuthScope                    string = "auth-scope"
	AnnotationAuthSessionCookie            string = "auth-session-cookie"
	AnnotationAuthSessionTimeout           string = "auth-session-timeout"
	AnnotationAuthOnUnauthenticatedRequest string = "auth-on-unauthenticated-request"
	AnnotationAuthIDPCognito               string = "auth-idp-cognito"
	AnnotationAuthIDPOIDC                  string = "auth-idp-oidc"
)

const (
	DefaultAuthType                     = TypeNone
	DefaultAuthScope                    = "openid"
	DefaultAuthSessionCookie            = "AWSELBAuthSessionCookie"
	DefaultAuthSessionTimeout           = 604800
	DefaultAuthOnUnauthenticatedRequest = OnUnauthenticatedRequestAuthenticate
)

const FieldAuthOIDCSecret = "authOIDCSecret"

// Authentication module interface
type Module interface {
	// Init setup index & watch functionality.
	Init(controller controller.Controller, ingressChan chan<- event.GenericEvent, serviceChan chan<- event.GenericEvent) error

	// NewConfig builds authentication config for ingress & ingressBackend.
	NewConfig(ctx context.Context, ingress *extensions.Ingress, backend extensions.IngressBackend, protocol string) (Config, error)
}

// NewModule constructs new Authentication module
func NewModule(cache cache.Cache) Module {
	return &defaultModule{
		cache: cache,
	}
}

type defaultModule struct {
	cache cache.Cache
}

func (m *defaultModule) Init(controller controller.Controller, ingressChan chan<- event.GenericEvent, serviceChan chan<- event.GenericEvent) error {
	if err := m.cache.IndexField(&extensions.Ingress{}, FieldAuthOIDCSecret, func(obj runtime.Object) []string {
		ingress := obj.(*extensions.Ingress)
		return buildOIDCSecretIndex(ingress.Namespace, ingress.Annotations)
	}); err != nil {
		return err
	}
	if err := m.cache.IndexField(&corev1.Service{}, FieldAuthOIDCSecret, func(obj runtime.Object) []string {
		service := obj.(*corev1.Service)
		return buildOIDCSecretIndex(service.Namespace, service.Annotations)
	}); err != nil {
		return err
	}

	if err := controller.Watch(&source.Kind{Type: &corev1.Secret{}}, &EnqueueRequestsForSecretEvent{
		IngressChan: ingressChan,
		ServiceChan: serviceChan,
		Cache:       m.cache,
	}); err != nil {
		return err
	}

	return nil
}

func (m *defaultModule) NewConfig(ctx context.Context, ingress *extensions.Ingress, backend extensions.IngressBackend, protocol string) (Config, error) {
	if protocol != elbv2.ProtocolEnumHttps {
		return Config{
			Type: TypeNone,
		}, nil
	}

	cfg := Config{
		Type:                     DefaultAuthType,
		OnUnauthenticatedRequest: DefaultAuthOnUnauthenticatedRequest,
		Scope:                    DefaultAuthScope,
		SessionCookie:            DefaultAuthSessionCookie,
		SessionTimeout:           DefaultAuthSessionTimeout,
	}

	ingressAnnos := ingress.Annotations
	var serviceAnnos map[string]string
	if !action.Use(backend.ServicePort.String()) {
		serviceKey := types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      backend.ServiceName,
		}
		service := corev1.Service{}
		if err := m.cache.Get(ctx, serviceKey, &service); err != nil {
			return Config{}, errors.Wrapf(err, "failed to get service %v", serviceKey)
		}
		serviceAnnos = service.Annotations
	}
	_ = annotations.LoadStringAnnotation(AnnotationAuthType, (*string)(&cfg.Type), serviceAnnos, ingressAnnos)
	_ = annotations.LoadStringAnnotation(AnnotationAuthOnUnauthenticatedRequest, (*string)(&cfg.OnUnauthenticatedRequest), serviceAnnos, ingressAnnos)
	_ = annotations.LoadStringAnnotation(AnnotationAuthScope, &cfg.Scope, serviceAnnos, ingressAnnos)
	_ = annotations.LoadStringAnnotation(AnnotationAuthSessionCookie, &cfg.SessionCookie, serviceAnnos, ingressAnnos)
	if _, err := annotations.LoadInt64Annotation(AnnotationAuthSessionTimeout, &cfg.SessionTimeout, serviceAnnos, ingressAnnos); err != nil {
		return Config{}, err
	}
	switch cfg.Type {
	case TypeCognito:
		{
			exists, err := annotations.LoadJSONAnnotation(AnnotationAuthIDPCognito, &cfg.IDPCognito, serviceAnnos, ingressAnnos)
			if err != nil {
				return Config{}, err
			}
			if !exists {
				return Config{}, errors.New(fmt.Sprintf("annotation %s is required when authType == %s", AnnotationAuthIDPCognito, TypeCognito))
			}
		}
	case TypeOIDC:
		{
			exists, err := m.loadIDPOIDC(ctx, &cfg.IDPOIDC, ingress.Namespace, serviceAnnos, ingressAnnos)
			if err != nil {
				return Config{}, err
			}
			if !exists {
				return Config{}, errors.New(fmt.Sprintf("annotation %s is required when authType == %s", AnnotationAuthIDPOIDC, TypeOIDC))
			}
		}
	}

	return cfg, nil
}

func (m *defaultModule) loadIDPOIDC(ctx context.Context, idpOIDC *IDPOIDC, namespace string, serviceAnnos map[string]string, ingressAnnos map[string]string) (bool, error) {
	annoIDPOIDC := AnnotationSchemaIDPOIDC{}
	exists, err := annotations.LoadJSONAnnotation(AnnotationAuthIDPOIDC, &annoIDPOIDC, serviceAnnos, ingressAnnos)
	if err != nil {
		return true, errors.Wrapf(err, "failed to load configuration for IDP OIDC")
	}
	if !exists {
		return false, nil
	}

	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      annoIDPOIDC.SecretName,
	}
	k8sSecret := corev1.Secret{}
	if err := m.cache.Get(ctx, secretKey, &k8sSecret); err != nil {
		return true, errors.Wrapf(err, "failed to load k8s secret: %v", secretKey)
	}
	clientId := strings.TrimRightFunc(string(k8sSecret.Data["clientId"]), unicode.IsSpace)
	clientSecret := string(k8sSecret.Data["clientSecret"])
	*idpOIDC = IDPOIDC{
		AuthenticationRequestExtraParams: annoIDPOIDC.AuthenticationRequestExtraParams,
		AuthorizationEndpoint:            annoIDPOIDC.AuthorizationEndpoint,
		Issuer:                           annoIDPOIDC.Issuer,
		TokenEndpoint:                    annoIDPOIDC.TokenEndpoint,
		UserInfoEndpoint:                 annoIDPOIDC.UserInfoEndpoint,
		ClientId:                         clientId,
		ClientSecret:                     clientSecret,
	}
	return true, nil
}

func buildOIDCSecretIndex(namespace string, annos map[string]string) []string {
	annoIDPOIDC := AnnotationSchemaIDPOIDC{}
	exists, err := annotations.LoadJSONAnnotation(AnnotationAuthIDPOIDC, &annoIDPOIDC, annos)
	if !exists || err != nil {
		return nil
	}

	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      annoIDPOIDC.SecretName,
	}.String()
	return []string{secretKey}
}
