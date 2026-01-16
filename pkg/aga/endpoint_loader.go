package aga

import (
	"context"
	"errors"
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// DNSLoadBalancerResolverInterface defines the interface for DNS resolvers
type DNSLoadBalancerResolverInterface interface {
	ResolveDNSToLoadBalancerARN(ctx context.Context, dnsName string) (string, error)
}

// DNSExtractorFunc extracts a DNS name from a Kubernetes object
type DNSExtractorFunc func(obj client.Object) (string, error)

// ResourceCreatorFunc creates a new instance of a specific Kubernetes resource
type ResourceCreatorFunc func() client.Object

// LoadedEndpointStatus represents the status of an endpoint loading operation
type LoadedEndpointStatus string

const (
	// EndpointStatusLoaded indicates the endpoint was successfully loaded with an ARN
	EndpointStatusLoaded LoadedEndpointStatus = "Loaded"

	// EndpointStatusWarning indicates the endpoint couldn't be loaded due to a non-fatal issue
	EndpointStatusWarning LoadedEndpointStatus = "Warning"

	// EndpointStatusFatal indicates the endpoint couldn't be loaded due to a fatal issue
	EndpointStatusFatal LoadedEndpointStatus = "Fatal"

	// Default Endpoint Weight
	DEFAULT_ENDPOINT_WEIGHT = 128
)

// LoadedEndpoint contains the resolved information for an endpoint
type LoadedEndpoint struct {
	// Original reference info
	Type        agaapi.GlobalAcceleratorEndpointType
	Name        string
	Namespace   string
	Weight      int32
	EndpointRef *agaapi.GlobalAcceleratorEndpoint

	// Resolved info (may be empty if loading failed)
	ARN     string // Load balancer ARN
	DNSName string // Original DNS name

	// Status and error info
	Status  LoadedEndpointStatus
	Error   error  // The error that occurred during loading, if any
	Message string // Human-readable message explaining the status

	// K8s resource reference - used for port and protocol discovery
	K8sResource client.Object

	// Cross-namespace permission - true if this is a cross-namespace reference that was allowed by a ReferenceGrant
	CrossNamespaceAllowed bool
}

// IsUsable returns true if this endpoint can be used in the model
func (e *LoadedEndpoint) IsUsable() bool {
	return e.Status == EndpointStatusLoaded
}

// GetKey generates a unique key for the endpoint
func (e *LoadedEndpoint) GetKey() string {
	if e.Type == agaapi.GlobalAcceleratorEndpointTypeEndpointID {
		return fmt.Sprintf("%s/%s", e.Type, e.ARN)
	}
	return fmt.Sprintf("%s/%s/%s", e.Type, e.Namespace, e.Name)
}

// EndpointLoader handles loading of GlobalAccelerator endpoints
type EndpointLoader interface {
	// LoadEndpoint loads a single endpoint and attempts to resolve its ARN
	// Always returns a LoadedEndpoint, even for failures
	LoadEndpoint(ctx context.Context, endpoint *agaapi.GlobalAcceleratorEndpoint, defaultNamespace string) *LoadedEndpoint

	// LoadEndpoints loads all endpoints from a GlobalAccelerator
	// Returns all endpoints (successful and failed) and any fatal errors
	LoadEndpoints(ctx context.Context, ga *agaapi.GlobalAccelerator, endpoints []EndpointReference) ([]*LoadedEndpoint, []error)
}

// endpointLoaderImpl implements the EndpointLoader interface
type endpointLoaderImpl struct {
	k8sClient               client.Client
	dnsResolver             DNSLoadBalancerResolverInterface
	logger                  logr.Logger
	crossNamespaceValidator CrossNamespaceValidator
}

// NewEndpointLoader creates a new EndpointLoader
func NewEndpointLoader(k8sClient client.Client, dnsResolver DNSLoadBalancerResolverInterface, logger logr.Logger, validator CrossNamespaceValidator) EndpointLoader {
	return &endpointLoaderImpl{
		k8sClient:               k8sClient,
		dnsResolver:             dnsResolver,
		logger:                  logger,
		crossNamespaceValidator: validator,
	}
}

// LoadEndpoint loads a single endpoint and attempts to resolve its ARN
func (l *endpointLoaderImpl) LoadEndpoint(ctx context.Context, endpoint *agaapi.GlobalAcceleratorEndpoint, defaultNamespace string) *LoadedEndpoint {
	namespace := defaultNamespace
	if endpoint.Namespace != nil {
		namespace = *endpoint.Namespace
	}

	// Set up the default result with basic information
	name := ""
	if endpoint.Name != nil {
		name = *endpoint.Name
	}

	weight := int32(DEFAULT_ENDPOINT_WEIGHT) // Default weight
	if endpoint.Weight != nil {
		weight = *endpoint.Weight
	}

	result := &LoadedEndpoint{
		Type:        endpoint.Type,
		Name:        name,
		Namespace:   namespace,
		Weight:      weight,
		EndpointRef: endpoint.DeepCopy(),
		Status:      EndpointStatusLoaded, // Default to success, will be changed if an error occurs
	}

	// Process based on endpoint type
	var err error

	switch endpoint.Type {
	case agaapi.GlobalAcceleratorEndpointTypeService:
		err = l.loadServiceEndpoint(ctx, result, defaultNamespace)
	case agaapi.GlobalAcceleratorEndpointTypeIngress:
		err = l.loadIngressEndpoint(ctx, result, defaultNamespace)
	case agaapi.GlobalAcceleratorEndpointTypeGateway:
		err = l.loadGatewayEndpoint(ctx, result, defaultNamespace)
	case agaapi.GlobalAcceleratorEndpointTypeEndpointID:
		err = l.loadEndpointIDEndpoint(ctx, result, defaultNamespace)
	default:
		err = NewFatalError(UnsupportedEndpointTypeMsg,
			fmt.Errorf("unsupported endpoint type: %s", endpoint.Type), endpoint, defaultNamespace)
	}

	// Handle any errors that occurred
	if err != nil {
		result.Error = err

		if IsFatal(err) {
			result.Status = EndpointStatusFatal
			var endpointErr *EndpointLoadError
			if errors.As(err, &endpointErr) {
				result.Message = endpointErr.Message
			} else {
				result.Message = err.Error()
			}
		} else {
			result.Status = EndpointStatusWarning
			var endpointErr *EndpointLoadError
			if errors.As(err, &endpointErr) {
				result.Message = endpointErr.Message
			} else {
				result.Message = err.Error()
			}
		}
	}

	return result
}

// loadResourceWithDNS is a generic resource loader using function parameters
func (l *endpointLoaderImpl) loadResourceWithDNS(
	ctx context.Context,
	result *LoadedEndpoint,
	parentNamespace string,
	resourceType string,
	createFunc ResourceCreatorFunc,
	extractDNSFunc DNSExtractorFunc,
) error {
	// Check for cross-namespace reference
	if result.Namespace != parentNamespace {
		// Get the appropriate group and kind based on resourceType
		var group, kind string
		switch resourceType {
		case string(ServiceResourceType):
			group = shared_constants.CoreAPIGroup
			kind = shared_constants.ServiceKind
		case string(IngressResourceType):
			group = shared_constants.IngressAPIGroup
			kind = shared_constants.IngressKind
		case string(GatewayResourceType):
			group = shared_constants.GatewayAPIResourcesGroup
			kind = shared_constants.GatewayApiKind
		default:
			return NewFatalError(UnsupportedResourceTypeMsg,
				fmt.Errorf("unsupported resource type: %s", resourceType),
				result.EndpointRef, parentNamespace)
		}

		// Validate cross-namespace reference using ReferenceGrant
		if err := l.crossNamespaceValidator.ValidateCrossNamespaceReference(ctx,
			parentNamespace,
			group, kind, result.Namespace, result.Name); err != nil {

			// Return a warning error if reference is not allowed
			return NewWarningError(CrossNamespaceReferenceMsg, err, result.EndpointRef, parentNamespace)
		}

		// If we got here, the reference is allowed - mark it in the result
		result.CrossNamespaceAllowed = true

		l.logger.V(1).Info("Cross-namespace reference allowed by ReferenceGrant",
			"from", fmt.Sprintf("%s/%s", parentNamespace, "GlobalAccelerator"),
			"to", fmt.Sprintf("%s/%s/%s", resourceType, result.Namespace, result.Name))
	}

	// Create object of the right type
	obj := createFunc()

	// Get resource
	err := l.k8sClient.Get(ctx, types.NamespacedName{Namespace: result.Namespace, Name: result.Name}, obj)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return NewWarningError(EndpointNotFoundMsg, err, result.EndpointRef, parentNamespace)
		}
		return NewFatalError(APIServerErrorMsg, err, result.EndpointRef, parentNamespace)
	}

	// Extract DNS name
	dnsName, err := extractDNSFunc(obj)
	if err != nil {
		return NewWarningError(LoadBalancerNotFoundMsg, err, result.EndpointRef, parentNamespace)
	}

	// Resolve DNS to ARN
	arn, err := l.dnsResolver.ResolveDNSToLoadBalancerARN(ctx, dnsName)
	if err != nil {
		// DNS resolution failure - warning
		return NewWarningError(DNSResolutionFailedMsg,
			fmt.Errorf("failed to resolve DNS name %s to ARN: %w", dnsName, err),
			result.EndpointRef, parentNamespace)
	}

	// Set the resolved information
	result.DNSName = dnsName
	result.ARN = arn
	result.Message = fmt.Sprintf("Successfully resolved %s to LoadBalancer ARN", resourceType)

	// Store the K8s resource object in the result as a generalized client.Object
	// This is used for port and protocol auto-discovery
	result.K8sResource = obj.DeepCopyObject().(client.Object)

	return nil
}

// extractServiceDNS extracts DNS from Services
func extractServiceDNS(obj client.Object) (string, error) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return "", fmt.Errorf("object is not a Service")
	}

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return "", fmt.Errorf("service %v is not of type LoadBalancer", k8s.NamespacedName(svc))
	}

	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return "", fmt.Errorf("service %v does not have a LoadBalancer", k8s.NamespacedName(svc))
	}

	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.Hostname != "" {
			return ingress.Hostname, nil
		}
	}

	return "", fmt.Errorf("service %v LoadBalancer has no DNS name", k8s.NamespacedName(svc))
}

// extractIngressDNS extracts DNS from Ingress
func extractIngressDNS(obj client.Object) (string, error) {
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return "", fmt.Errorf("object is not an Ingress")
	}

	if len(ing.Status.LoadBalancer.Ingress) == 0 {
		return "", fmt.Errorf("ingress %v does not have a LoadBalancer", k8s.NamespacedName(ing))
	}

	albDNS, _ := shared_utils.FindIngressTwoDNSName(ing)
	if albDNS != "" {
		return albDNS, nil
	}

	return "", fmt.Errorf("ingress %v LoadBalancer has no DNS name", k8s.NamespacedName(ing))
}

// extractGatewayDNS extracts DNS from Gateway
func extractGatewayDNS(obj client.Object) (string, error) {
	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return "", fmt.Errorf("object is not a Gateway")
	}

	if len(gw.Status.Addresses) == 0 {
		return "", fmt.Errorf("gateway %v does not have any addresses", k8s.NamespacedName(gw))
	}

	for _, addr := range gw.Status.Addresses {
		if addr.Type != nil && *addr.Type == gwv1.HostnameAddressType && addr.Value != "" {
			return addr.Value, nil
		}
	}

	return "", fmt.Errorf("gateway %v has no hostname address", k8s.NamespacedName(gw))
}

// loadServiceEndpoint loads service details into the provided LoadedEndpoint
func (l *endpointLoaderImpl) loadServiceEndpoint(ctx context.Context, result *LoadedEndpoint, parentNamespace string) error {
	return l.loadResourceWithDNS(
		ctx,
		result,
		parentNamespace,
		string(ServiceResourceType),
		func() client.Object { return &corev1.Service{} },
		extractServiceDNS,
	)
}

// loadIngressEndpoint loads ingress details into the provided LoadedEndpoint
func (l *endpointLoaderImpl) loadIngressEndpoint(ctx context.Context, result *LoadedEndpoint, parentNamespace string) error {
	return l.loadResourceWithDNS(
		ctx,
		result,
		parentNamespace,
		string(IngressResourceType),
		func() client.Object { return &networkingv1.Ingress{} },
		extractIngressDNS,
	)
}

// loadGatewayEndpoint loads gateway details into the provided LoadedEndpoint
func (l *endpointLoaderImpl) loadGatewayEndpoint(ctx context.Context, result *LoadedEndpoint, parentNamespace string) error {
	return l.loadResourceWithDNS(
		ctx,
		result,
		parentNamespace,
		string(GatewayResourceType),
		func() client.Object { return &gwv1.Gateway{} },
		extractGatewayDNS,
	)
}

// loadEndpointIDEndpoint loads direct ARN endpoint info
func (l *endpointLoaderImpl) loadEndpointIDEndpoint(_ context.Context, result *LoadedEndpoint, parentNamespace string) error {
	if result.EndpointRef.EndpointID == nil || *result.EndpointRef.EndpointID == "" {
		return NewFatalError(EndpointIDEmptyMsg,
			fmt.Errorf("endpointID is required for endpoint type EndpointID"),
			result.EndpointRef, parentNamespace)
	}

	result.ARN = *result.EndpointRef.EndpointID
	result.Message = "Using provided EndpointID directly"

	return nil
}

// LoadEndpoints loads all endpoints from a GlobalAccelerator
func (l *endpointLoaderImpl) LoadEndpoints(ctx context.Context, ga *agaapi.GlobalAccelerator, endpoints []EndpointReference) ([]*LoadedEndpoint, []error) {
	var loadedEndpoints []*LoadedEndpoint
	var fatalErrors []error

	for _, endpoint := range endpoints {
		// Access the GlobalAcceleratorEndpoint from the EndpointReference
		if endpoint.Endpoint == nil {
			// This should never happen, but handle it gracefully
			l.logger.Error(nil, "Nil endpoint reference found", "endpoint", endpoint)
			continue
		}
		loadedEndpoint := l.LoadEndpoint(ctx, endpoint.Endpoint, ga.Namespace)

		// Add to the result list regardless of status
		loadedEndpoints = append(loadedEndpoints, loadedEndpoint)

		// Log and collect errors
		if loadedEndpoint.Status == EndpointStatusFatal {
			l.logger.Error(loadedEndpoint.Error, "Fatal error loading endpoint",
				"globalAccelerator", k8s.NamespacedName(ga),
				"endpointType", endpoint.Type,
				"endpointName", endpoint.Name,
				"message", loadedEndpoint.Message)
			fatalErrors = append(fatalErrors, loadedEndpoint.Error)
		} else if loadedEndpoint.Status == EndpointStatusWarning {
			l.logger.Info("Warning while loading endpoint",
				"globalAccelerator", k8s.NamespacedName(ga),
				"error", loadedEndpoint.Error,
				"message", loadedEndpoint.Message,
				"endpointType", endpoint.Type,
				"endpointName", endpoint.Name)
		}
	}

	// Temporary
	LogAllEndpoints(l.logger, loadedEndpoints, ga)

	return loadedEndpoints, fatalErrors
}

// LogEndpointDetails logs detailed information about a loaded endpoint
func LogEndpointDetails(logger logr.Logger, endpoint *LoadedEndpoint) {
	logger.V(1).Info("Endpoint details",
		"type", endpoint.Type,
		"name", endpoint.Name,
		"namespace", endpoint.Namespace,
		"status", endpoint.Status,
		"weight", endpoint.Weight,
		"dnsName", endpoint.DNSName,
		"arn", endpoint.ARN,
		"message", endpoint.Message)

	if endpoint.Error != nil {
		logger.V(1).Info("Endpoint error details",
			"error", endpoint.Error.Error(),
			"type", endpoint.Type,
			"name", endpoint.Name)
	}
}

// LogAllEndpoints logs information for a collection of endpoints
func LogAllEndpoints(logger logr.Logger, endpoints []*LoadedEndpoint, ga *agaapi.GlobalAccelerator) {
	logger.V(1).Info("===== ENDPOINT LOADING SUMMARY =====",
		"globalAccelerator", k8s.NamespacedName(ga))
	var loaded, warning, fatal int

	for _, endpoint := range endpoints {
		switch endpoint.Status {
		case EndpointStatusLoaded:
			loaded++
		case EndpointStatusWarning:
			warning++
		case EndpointStatusFatal:
			fatal++
		}
	}

	logger.V(1).Info("Endpoint loading statistics",
		"total", len(endpoints),
		"loaded", loaded,
		"warnings", warning,
		"fatal", fatal)

	// Log individual endpoints
	for i, endpoint := range endpoints {
		logger.V(1).Info(fmt.Sprintf("Endpoint %d of %d", i+1, len(endpoints)))
		LogEndpointDetails(logger, endpoint)
	}
	logger.V(1).Info("===== END ENDPOINT LOADING SUMMARY =====",
		"globalAccelerator", k8s.NamespacedName(ga))
}
