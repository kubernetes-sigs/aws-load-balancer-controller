package ingress

import (
	"context"
	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// IndexKey for services referenced by Ingress.
	IndexKeyServiceRefName = "ingress.serviceRef.name"
	// IndexKey for secrets referenced by Ingress or Service.
	IndexKeySecretRefName = "ingress.secretRef.name"
)

// ReferenceIndexer has the ability to index Ingresses with referenced objects.
type ReferenceIndexer interface {
	BuildServiceRefIndexes(ctx context.Context, ing *networking.Ingress) []string
	BuildSecretRefIndexes(ctx context.Context, ingOrSvc metav1.Object) []string
}

// NewDefaultReferenceIndexer constructs new defaultReferenceIndexer.
func NewDefaultReferenceIndexer(enhancedBackendBuilder EnhancedBackendBuilder, authConfigBuilder AuthConfigBuilder, logger logr.Logger) *defaultReferenceIndexer {
	return &defaultReferenceIndexer{
		enhancedBackendBuilder: enhancedBackendBuilder,
		authConfigBuilder:      authConfigBuilder,
		logger:                 logger,
	}
}

var _ ReferenceIndexer = &defaultReferenceIndexer{}

// default implementation for ReferenceIndexer
type defaultReferenceIndexer struct {
	enhancedBackendBuilder EnhancedBackendBuilder
	authConfigBuilder      AuthConfigBuilder
	logger                 logr.Logger
}

func (i *defaultReferenceIndexer) BuildServiceRefIndexes(ctx context.Context, ing *networking.Ingress) []string {
	var backends []networking.IngressBackend
	if ing.Spec.Backend != nil {
		backends = append(backends, *ing.Spec.Backend)
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			backends = append(backends, path.Backend)
		}
	}

	serviceNames := sets.NewString()
	for _, backend := range backends {
		enhancedBackend, err := i.enhancedBackendBuilder.Build(ctx, ing, backend)
		if err != nil {
			i.logger.Error(err, "failed to build Ingress indexes",
				"indexKey", IndexKeyServiceRefName)
			return nil
		}
		serviceNamesFromBackend := extractServiceNamesFromAction(enhancedBackend.Action)
		serviceNames.Insert(serviceNamesFromBackend...)
	}
	return serviceNames.List()
}

func (i *defaultReferenceIndexer) BuildSecretRefIndexes(ctx context.Context, ingOrSvc metav1.Object) []string {
	authCfg, err := i.authConfigBuilder.Build(ctx, ingOrSvc.GetAnnotations())
	if err != nil {
		i.logger.Error(err, "failed to build Ingress indexes",
			"indexKey", IndexKeySecretRefName)
		return nil
	}
	return extractSecretNamesFromAuthConfig(authCfg)
}

func extractServiceNamesFromAction(action Action) []string {
	if action.Type != ActionTypeForward || action.ForwardConfig == nil {
		return nil
	}
	serviceNames := sets.NewString()
	for _, tgt := range action.ForwardConfig.TargetGroups {
		serviceNamesFromTGT := extractServiceNamesFromTargetGroupTuple(tgt)
		serviceNames.Insert(serviceNamesFromTGT...)
	}
	return serviceNames.List()
}

func extractServiceNamesFromTargetGroupTuple(tgt TargetGroupTuple) []string {
	if tgt.ServiceName == nil {
		return nil
	}
	return []string{*tgt.ServiceName}
}

func extractSecretNamesFromAuthConfig(authCfg AuthConfig) []string {
	if authCfg.IDPConfigOIDC == nil {
		return nil
	}
	return []string{authCfg.IDPConfigOIDC.SecretName}
}
