package gateway

import (
	"context"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// IndexKeyGatewayRefName is index key for Gateway
	IndexKeyGatewayRefName = "gateway.gatewayRef.name"
	// IndexKeyGatewayClassRefName is index key for GatewayClass referenced by Gateway.
	IndexKeyGatewayClassRefName = "gateway.gatewayClassRef.name"
	// IndexKeyGatewayClassParamsRefName is index key for GatewayClassParameters referenced by Gateway.
	IndexKeyGatewayClassParamsRefName = "gateway.gatewayClassParamsRef.name"
	// IndexKeyServiceRefName is index key for services referenced by Gateway.
	IndexKeyServiceRefName = "ingress.serviceRef.name"
	// IndexKeyTCPRouteRefName is index key for TCPRoute referenced by Gateway.
	IndexKeyTCPRouteRefName = "gateway.tcpRouteRef.name"
	// IndexKeySecretRefName is index key for secrets referenced by Ingress or Service.
	IndexKeySecretRefName = "ingress.secretRef.name"
)

// ReferenceIndexer has the ability to index Gateways with referenced objects.
// See - https://gateway-api.sigs.k8s.io/guides/tcp/
// Gateway Listener contains a PortNumber which is mapped to a k8s Service (BackendObjectReference) via TCPRoute
// * Find all the TCPRouteSpec with []ParentReference for this Gateway
// * match the sectionName in the parentRefs to a listener name in the Gateway
// * map the associated backendRefs service name and port
// * ToDo: check allowedRoutes to ensure it should be added
type ReferenceIndexer interface {

	// BuildTCPRouteRefIndexes returns the name of related TCPRoute objects.
	BuildTCPRouteRefIndexes(ctx context.Context, gateway *v1beta1.Gateway) []string
	// BuildServiceRefIndexes returns the name of related Service objects.
	BuildServiceRefIndexes(ctx context.Context, gateway *v1beta1.Gateway) []string
	// BuildSecretRefIndexes returns the name of related Secret objects.
	BuildSecretRefIndexes(ctx context.Context, ingOrSvc client.Object) []string
	// BuildGatewayClassRefIndexes returns the name of related GatewayClass objects.
	BuildGatewayClassRefIndexes(ctx context.Context, gateway *v1beta1.Gateway) []string
	// BuildGatewayClassParamsRefIndexes returns the name of related GatewayClassParams objects.
	BuildGatewayClassParamsRefIndexes(ctx context.Context, gatewayClass *v1beta1.GatewayClass) []string
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

func (i *defaultReferenceIndexer) BuildTCPRouteRefIndexes(ctx context.Context, ing *networking.Ingress) []string {
	var backends []networking.IngressBackend
	if ing.Spec.DefaultBackend != nil {
		backends = append(backends, *ing.Spec.DefaultBackend)
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
		enhancedBackend, err := i.enhancedBackendBuilder.Build(ctx, ing, backend,
			WithLoadBackendServices(false, nil),
			WithLoadAuthConfig(false),
		)
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

func (i *defaultReferenceIndexer) BuildServiceRefIndexes(ctx context.Context, ing *networking.Ingress) []string {
	var backends []networking.IngressBackend
	if ing.Spec.DefaultBackend != nil {
		backends = append(backends, *ing.Spec.DefaultBackend)
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
		enhancedBackend, err := i.enhancedBackendBuilder.Build(ctx, ing, backend,
			WithLoadBackendServices(false, nil),
			WithLoadAuthConfig(false),
		)
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

func (i *defaultReferenceIndexer) BuildSecretRefIndexes(ctx context.Context, ingOrSvc client.Object) []string {
	authCfg, err := i.authConfigBuilder.Build(ctx, ingOrSvc.GetAnnotations())
	if err != nil {
		i.logger.Error(err, "failed to build Ingress indexes",
			"indexKey", IndexKeySecretRefName)
		return nil
	}
	return extractSecretNamesFromAuthConfig(authCfg)
}

func (i *defaultReferenceIndexer) BuildIngressClassRefIndexes(_ context.Context, ing *networking.Ingress) []string {
	if ing.Spec.IngressClassName == nil {
		return nil
	}

	ingClassName := awssdk.StringValue(ing.Spec.IngressClassName)
	return []string{ingClassName}
}

func (i *defaultReferenceIndexer) BuildIngressClassParamsRefIndexes(_ context.Context, ingClass *networking.IngressClass) []string {
	if ingClass.Spec.Controller != ingressClassControllerALB || ingClass.Spec.Parameters == nil {
		return nil
	}
	if ingClass.Spec.Parameters.APIGroup == nil ||
		(*ingClass.Spec.Parameters.APIGroup) != elbv2api.GroupVersion.Group ||
		ingClass.Spec.Parameters.Kind != ingressClassParamsKind {
		return nil
	}
	ingClassParamsName := ingClass.Spec.Parameters.Name
	return []string{ingClassParamsName}
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
