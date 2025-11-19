package aga

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidateAGAGlobalAccelerator = "/validate-aga-k8s-aws-v1beta1-globalaccelerator"
)

// NewGlobalAcceleratorValidator returns a validator for GlobalAccelerator API.
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
	return &agaapi.GlobalAccelerator{}, nil
}

func (v *globalAcceleratorValidator) ValidateCreate(_ context.Context, obj runtime.Object) error {
	ga := obj.(*agaapi.GlobalAccelerator)

	if err := v.checkForOverlappingPortRanges(ga); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateAGAGlobalAccelerator, "checkForOverlappingPortRanges")
		return err
	}

	return nil
}

func (v *globalAcceleratorValidator) ValidateUpdate(_ context.Context, obj runtime.Object, _ runtime.Object) error {
	ga := obj.(*agaapi.GlobalAccelerator)

	if err := v.checkForOverlappingPortRanges(ga); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateAGAGlobalAccelerator, "checkForOverlappingPortRanges")
		return err
	}

	return nil
}

func (v *globalAcceleratorValidator) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

// checkForOverlappingPortRanges checks if there are overlapping port ranges across all listeners
// grouped by protocol
func (v *globalAcceleratorValidator) checkForOverlappingPortRanges(ga *agaapi.GlobalAccelerator) error {
	if ga.Spec.Listeners == nil {
		return nil
	}

	// Group all port ranges by protocol
	portRangesByProtocol := make(map[agaapi.GlobalAcceleratorProtocol][]agaapi.PortRange)

	// Process all listeners and collect port ranges by protocol
	for _, listener := range *ga.Spec.Listeners {
		if listener.PortRanges == nil || len(*listener.PortRanges) == 0 {
			continue
		}

		// Skip listeners with nil protocol, we will assign protocols based on endpoints
		if listener.Protocol == nil {
			continue
		}

		// Add all port ranges from this listener to the appropriate protocol group
		portRangesByProtocol[*listener.Protocol] = append(portRangesByProtocol[*listener.Protocol], *listener.PortRanges...)
	}

	// Check each protocol group for overlapping port ranges
	for protocol, portRanges := range portRangesByProtocol {
		if hasOverlappingRangesInSlice(portRanges) {
			return errors.Errorf(
				"overlapping port ranges detected for protocol %s, which is not allowed",
				protocol)
		}
	}

	return nil
}

// hasOverlappingRangesInSlice checks if there are any overlapping ranges within a slice of port ranges
func hasOverlappingRangesInSlice(portRanges []agaapi.PortRange) bool {
	for i := 0; i < len(portRanges); i++ {
		for j := i + 1; j < len(portRanges); j++ {
			if portRangesOverlap(portRanges[i], portRanges[j]) {
				return true
			}
		}
	}
	return false
}

// portRangesOverlap checks if two port ranges overlap
func portRangesOverlap(rangeA agaapi.PortRange, rangeB agaapi.PortRange) bool {
	// Ranges overlap if start of A is before or at end of B AND end of A is after or at start of B
	return rangeA.FromPort <= rangeB.ToPort && rangeA.ToPort >= rangeB.FromPort
}

// +kubebuilder:webhook:path=/validate-aga-k8s-aws-v1beta1-globalaccelerator,mutating=false,failurePolicy=fail,groups=aga.k8s.aws,resources=globalaccelerators,verbs=create;update,versions=v1beta1,name=vglobalaccelerator.aga.k8s.aws,sideEffects=None,matchPolicy=Equivalent,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *globalAcceleratorValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateAGAGlobalAccelerator, webhook.ValidatingWebhookForValidator(v, mgr.GetScheme()))
}
