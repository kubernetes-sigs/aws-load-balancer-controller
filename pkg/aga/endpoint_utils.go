package aga

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"k8s.io/apimachinery/pkg/types"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
)

// ResourceType defines the type of resource that can be referenced by a GlobalAccelerator
type ResourceType string

const (
	// ServiceResourceType represents a Service resource
	ServiceResourceType ResourceType = "Service"
	// IngressResourceType represents an Ingress resource
	IngressResourceType ResourceType = "Ingress"
	// GatewayResourceType represents a Gateway resource
	GatewayResourceType ResourceType = "Gateway"
)

// EndpointReference contains information about a referenced endpoint
type EndpointReference struct {
	Type       agaapi.GlobalAcceleratorEndpointType
	Name       string // Used for Service/Ingress/Gateway type endpoints
	Namespace  string // Used for Service/Ingress/Gateway type endpoints
	EndpointID string // Used for EndpointID type endpoints (ARN of LB or other resources)
	Endpoint   *agaapi.GlobalAcceleratorEndpoint
}

// GetAllDesiredEndpointsFromGA extracts all endpoint references from a GlobalAccelerator resource
func GetAllDesiredEndpointsFromGA(ga *agaapi.GlobalAccelerator) []EndpointReference {
	if ga == nil || ga.Spec.Listeners == nil {
		return nil
	}

	var endpoints []EndpointReference

	for _, listener := range *ga.Spec.Listeners {
		if listener.EndpointGroups == nil {
			continue
		}

		for _, endpointGroup := range *listener.EndpointGroups {
			if endpointGroup.Endpoints == nil {
				continue
			}

			for _, endpoint := range *endpointGroup.Endpoints {
				var name, namespace, endpointID string

				if endpoint.Type == agaapi.GlobalAcceleratorEndpointTypeEndpointID {
					// For EndpointID type, the endpointID will be set according to CRD validation
					endpointID = awssdk.ToString(endpoint.EndpointID)
					// For EndpointID type, name and namespace must not be set
					name = ""
					namespace = ""
				} else {
					// For Service/Ingress/Gateway types, name will be set according to CRD validation
					name = awssdk.ToString(endpoint.Name)

					// Determine namespace
					namespace = ga.Namespace
					// We allow the namespace to be specified, but will handle cross-namespace references
					// as warnings in the endpoint loader
					if endpoint.Namespace != nil && *endpoint.Namespace != "" {
						namespace = *endpoint.Namespace
					}

					// For these types, endpointID must not be set
					endpointID = ""
				}

				// Add to list - we want all endpoints regardless of type
				endpoints = append(endpoints, EndpointReference{
					Type:       endpoint.Type,
					Name:       name,
					Namespace:  namespace,
					EndpointID: endpointID,
					Endpoint:   &endpoint,
				})
			}
		}
	}

	return endpoints
}

// ToResourceKey converts an EndpointReference to a ResourceKey for the reference tracker
func (e EndpointReference) ToResourceKey() ResourceKey {
	switch e.Type {
	case agaapi.GlobalAcceleratorEndpointTypeEndpointID:
		// For EndpointID type, use the EndpointID as the resource name
		// We'll use an empty namespace since EndpointIDs are not namespaced
		return ResourceKey{
			Type: ResourceType(e.Type),
			Name: types.NamespacedName{
				Namespace: "",
				Name:      e.EndpointID,
			},
		}
	default:
		// For Service/Ingress/Gateway, use Name and Namespace
		return ResourceKey{
			Type: ResourceType(e.Type),
			Name: types.NamespacedName{
				Namespace: e.Namespace,
				Name:      e.Name,
			},
		}
	}
}
