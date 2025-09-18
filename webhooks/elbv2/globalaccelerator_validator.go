package elbv2

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidateELBv2GlobalAccelerator = "/validate-elbv2-k8s-aws-v1beta1-globalaccelerator"
)

// NewGlobalAcceleratorValidator returns a validator for GlobalAccelerator CRD.
func NewGlobalAcceleratorValidator(logger logr.Logger, metricsCollector lbcmetrics.MetricCollector) *globalAcceleratorValidator {
	return &globalAcceleratorValidator{
		logger:           logger,
		metricsCollector: metricsCollector,
	}
}

var _ webhook.Validator = &globalAcceleratorValidator{}

type globalAcceleratorValidator struct {
	logger           logr.Logger
	metricsCollector lbcmetrics.MetricCollector
}

func (v *globalAcceleratorValidator) Prototype(req admission.Request) (runtime.Object, error) {
	return &elbv2api.GlobalAccelerator{}, nil
}

func (v *globalAcceleratorValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	ga := obj.(*elbv2api.GlobalAccelerator)
	return v.validateGlobalAccelerator(ctx, ga)
}

func (v *globalAcceleratorValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	ga := obj.(*elbv2api.GlobalAccelerator)
	return v.validateGlobalAccelerator(ctx, ga)
}

func (v *globalAcceleratorValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	// No validation needed for delete operations
	return nil
}

func (v *globalAcceleratorValidator) validateGlobalAccelerator(ctx context.Context, ga *elbv2api.GlobalAccelerator) error {
	if err := v.validateSpec(ga.Spec); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2GlobalAccelerator, "validateSpec")
		return err
	}
	return nil
}

func (v *globalAcceleratorValidator) validateSpec(spec elbv2api.GlobalAcceleratorSpec) error {
	// Validate listeners
	if len(spec.Listeners) == 0 {
		return fmt.Errorf("at least one listener is required")
	}

	// Validate endpoint groups
	if len(spec.EndpointGroups) == 0 {
		return fmt.Errorf("at least one endpoint group is required")
	}

	// Validate that we have endpoint groups for listeners
	if len(spec.EndpointGroups) < len(spec.Listeners) {
		return fmt.Errorf("number of endpoint groups (%d) must be at least the number of listeners (%d)",
			len(spec.EndpointGroups), len(spec.Listeners))
	}

	// Validate listeners
	for i, listener := range spec.Listeners {
		if err := v.validateListener(listener, i); err != nil {
			return err
		}
	}

	// Validate endpoint groups
	for i, endpointGroup := range spec.EndpointGroups {
		if err := v.validateEndpointGroup(endpointGroup, i); err != nil {
			return err
		}
	}

	// Validate service endpoints if specified
	if len(spec.ServiceEndpoints) > 0 {
		for i, svcEndpoint := range spec.ServiceEndpoints {
			if err := v.validateServiceEndpoint(svcEndpoint, i); err != nil {
				return err
			}
		}
	}

	return nil
}

func (v *globalAcceleratorValidator) validateListener(listener elbv2api.GlobalAcceleratorListener, index int) error {
	// Validate protocol
	if listener.Protocol != elbv2api.GlobalAcceleratorProtocolTCP &&
		listener.Protocol != elbv2api.GlobalAcceleratorProtocolUDP {
		return fmt.Errorf("listener[%d]: invalid protocol %s, must be TCP or UDP", index, listener.Protocol)
	}

	// Validate port ranges
	if len(listener.PortRanges) == 0 {
		return fmt.Errorf("listener[%d]: at least one port range is required", index)
	}

	for i, portRange := range listener.PortRanges {
		if portRange.FromPort < 1 || portRange.FromPort > 65535 {
			return fmt.Errorf("listener[%d].portRanges[%d]: fromPort must be between 1 and 65535", index, i)
		}
		if portRange.ToPort < 1 || portRange.ToPort > 65535 {
			return fmt.Errorf("listener[%d].portRanges[%d]: toPort must be between 1 and 65535", index, i)
		}
		if portRange.FromPort > portRange.ToPort {
			return fmt.Errorf("listener[%d].portRanges[%d]: fromPort (%d) cannot be greater than toPort (%d)",
				index, i, portRange.FromPort, portRange.ToPort)
		}
	}

	return nil
}

func (v *globalAcceleratorValidator) validateEndpointGroup(endpointGroup elbv2api.EndpointGroup, index int) error {
	// Validate region
	if strings.TrimSpace(endpointGroup.Region) == "" {
		return fmt.Errorf("endpointGroups[%d]: region is required", index)
	}

	// Validate that we have endpoints (either explicit or will be resolved from services)
	if len(endpointGroup.Endpoints) == 0 {
		return fmt.Errorf("endpointGroups[%d]: at least one endpoint is required", index)
	}

	// Validate endpoints
	for i, endpoint := range endpointGroup.Endpoints {
		if strings.TrimSpace(endpoint.EndpointID) == "" {
			return fmt.Errorf("endpointGroups[%d].endpoints[%d]: endpointID is required", index, i)
		}
		if endpoint.Weight != nil && (*endpoint.Weight < 0 || *endpoint.Weight > 255) {
			return fmt.Errorf("endpointGroups[%d].endpoints[%d]: weight must be between 0 and 255", index, i)
		}
	}

	// Validate traffic dial percentage
	if endpointGroup.TrafficDialPercentage != nil {
		percentage := *endpointGroup.TrafficDialPercentage
		if percentage < 0 || percentage > 100 {
			return fmt.Errorf("endpointGroups[%d]: trafficDialPercentage must be between 0 and 100", index)
		}
	}

	// Validate health check settings
	if endpointGroup.HealthCheckIntervalSeconds != nil {
		interval := *endpointGroup.HealthCheckIntervalSeconds
		if interval < 10 || interval > 30 {
			return fmt.Errorf("endpointGroups[%d]: healthCheckIntervalSeconds must be between 10 and 30", index)
		}
	}

	if endpointGroup.ThresholdCount != nil {
		threshold := *endpointGroup.ThresholdCount
		if threshold < 1 || threshold > 10 {
			return fmt.Errorf("endpointGroups[%d]: thresholdCount must be between 1 and 10", index)
		}
	}

	return nil
}

func (v *globalAcceleratorValidator) validateServiceEndpoint(svcEndpoint elbv2api.ServiceEndpointReference, index int) error {
	// Validate service name
	if strings.TrimSpace(svcEndpoint.Name) == "" {
		return fmt.Errorf("serviceEndpoints[%d]: name is required", index)
	}

	// Validate weight if specified
	if svcEndpoint.Weight != nil {
		weight := *svcEndpoint.Weight
		if weight < 0 || weight > 255 {
			return fmt.Errorf("serviceEndpoints[%d]: weight must be between 0 and 255", index)
		}
	}

	return nil
}

// SetupWithManager sets up the webhook with the Manager.
func (v *globalAcceleratorValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateELBv2GlobalAccelerator, webhook.ValidatingWebhookForValidator(v, mgr.GetScheme()))
}
