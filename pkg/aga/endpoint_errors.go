package aga

import (
	"errors"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
)

// EndpointLoadErrorType categorizes endpoint errors by severity
type EndpointLoadErrorType string

const (
	// ErrorTypeFatal indicates errors that should stop reconciliation
	ErrorTypeFatal EndpointLoadErrorType = "Fatal"

	// ErrorTypeWarning indicates errors that allow reconciliation to continue
	ErrorTypeWarning EndpointLoadErrorType = "Warning"
)

// EndpointLoadError represents an error encountered during endpoint loading
type EndpointLoadError struct {
	Type            EndpointLoadErrorType
	Message         string
	Err             error
	EndpointRef     *agaapi.GlobalAcceleratorEndpoint
	ParentNamespace string // The namespace of the parent GlobalAccelerator
}

// Error implements the error interface
func (e *EndpointLoadError) Error() string {
	endpointStr := "unknown"
	if e.EndpointRef != nil {
		if e.EndpointRef.Type == agaapi.GlobalAcceleratorEndpointTypeEndpointID {
			// For EndpointID type, we know endpointID is always non-nil
			endpointStr = fmt.Sprintf("%s/%s", e.EndpointRef.Type, awssdk.ToString(e.EndpointRef.EndpointID))
		} else {
			// For other types, we know name is always non-nil
			namespace := e.ParentNamespace // Use parent namespace as default
			if e.EndpointRef.Namespace != nil {
				namespace = *e.EndpointRef.Namespace
			}
			endpointStr = fmt.Sprintf("%s/%s/%s", e.EndpointRef.Type, namespace, awssdk.ToString(e.EndpointRef.Name))
		}
	}
	return fmt.Sprintf("%s error for endpoint %s: %s - %v", e.Type, endpointStr, e.Message, e.Err)
}

// Unwrap returns the underlying error
func (e *EndpointLoadError) Unwrap() error {
	return e.Err
}

// NewFatalError creates a new fatal endpoint error
func NewFatalError(message string, err error, endpoint *agaapi.GlobalAcceleratorEndpoint, parentNamespace string) *EndpointLoadError {
	return &EndpointLoadError{
		Type:            ErrorTypeFatal,
		Message:         message,
		Err:             err,
		EndpointRef:     endpoint,
		ParentNamespace: parentNamespace,
	}
}

// NewWarningError creates a new warning endpoint error
func NewWarningError(message string, err error, endpoint *agaapi.GlobalAcceleratorEndpoint, parentNamespace string) *EndpointLoadError {
	return &EndpointLoadError{
		Type:            ErrorTypeWarning,
		Message:         message,
		Err:             err,
		EndpointRef:     endpoint,
		ParentNamespace: parentNamespace,
	}
}

// IsFatal checks if an error is a fatal endpoint error
func IsFatal(err error) bool {
	var endpointErr *EndpointLoadError
	if errors.As(err, &endpointErr) {
		return endpointErr.Type == ErrorTypeFatal
	}
	return false
}

// IsWarning checks if an error is a warning endpoint error
func IsWarning(err error) bool {
	var endpointErr *EndpointLoadError
	if errors.As(err, &endpointErr) {
		return endpointErr.Type == ErrorTypeWarning
	}
	return false
}

// Constants for common error messages
const (
	EndpointNotFoundMsg        = "Referenced resource not found"
	LoadBalancerNotFoundMsg    = "Resource does not have a LoadBalancer"
	DNSResolutionFailedMsg     = "Failed to resolve DNS name to ARN"
	EndpointIDEmptyMsg         = "EndpointID is required for EndpointID type"
	UnsupportedEndpointTypeMsg = "Unsupported endpoint type"
	APIServerErrorMsg          = "Error contacting Kubernetes API server"
	CrossNamespaceReferenceMsg = "Cross-namespace reference denied"
)
