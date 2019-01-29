package tls

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"

	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/controller-runtime/pkg/cache"

	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	AnnotationSSLPolicy      = "ssl-policy"
	AnnotationCertificateARN = "certificate-arn"
)

const (
	DefaultSSLPolicy = "ELBSecurityPolicy-2016-08"
)

const FieldTLSSecret = "TLSSecret"

// TLS module interface
type Module interface {
	// Init setup index & watch functionality.
	Init(controller controller.Controller, ingressChan chan<- event.GenericEvent) error

	// NewConfig builds tls config for ingress.
	NewConfig(ctx context.Context, ingress *extensions.Ingress) (Config, error)
}

// NewModule constructs new Authentication module
func NewModule(cache cache.Cache) Module {
	certLoader := NewCertLoader(cache)
	return &defaultModule{
		cache:      cache,
		certLoader: certLoader,
	}
}

var _ Module = (*defaultModule)(nil)

type defaultModule struct {
	cache      cache.Cache
	certLoader CertLoader
}

func (m *defaultModule) Init(controller controller.Controller, ingressChan chan<- event.GenericEvent) error {
	if err := m.cache.IndexField(&extensions.Ingress{}, FieldTLSSecret, func(obj runtime.Object) []string {
		ingress := obj.(*extensions.Ingress)
		return buildTLSSecretIndex(ingress.Namespace, ingress.Spec.TLS)
	}); err != nil {
		return err
	}

	if err := controller.Watch(&source.Kind{Type: &corev1.Secret{}}, &EnqueueRequestsForSecretEvent{
		Cache:       m.cache,
		IngressChan: ingressChan,
	}); err != nil {
		return err
	}

	return nil
}

func (m *defaultModule) NewConfig(ctx context.Context, ing *extensions.Ingress) (Config, error) {
	cfg := Config{
		SSLPolicy: DefaultSSLPolicy,
	}
	_ = annotations.LoadStringAnnotation(AnnotationSSLPolicy, &cfg.SSLPolicy, ing.Annotations)
	_ = annotations.LoadStringSliceAnnotation(AnnotationCertificateARN, &cfg.ACMCertificates, ing.Annotations)
	for _, tlsSpec := range ing.Spec.TLS {
		if len(tlsSpec.SecretName) == 0 {
			continue
		}
		secretKey := types.NamespacedName{
			Namespace: ing.Namespace,
			Name:      tlsSpec.SecretName,
		}
		cert, err := m.certLoader.Load(ctx, secretKey)
		if err != nil {
			return Config{}, err
		}
		cfg.RawCertificates = append(cfg.RawCertificates, cert)
	}

	return cfg, nil
}

func buildTLSSecretIndex(namespace string, ingTLS []extensions.IngressTLS) []string {
	indexes := make([]string, 0, len(ingTLS))
	for _, tlsSpec := range ingTLS {
		secretKey := types.NamespacedName{
			Namespace: namespace,
			Name:      tlsSpec.SecretName,
		}
		indexes = append(indexes, secretKey.String())
	}
	return indexes
}
