package aga

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

const (
	agaController = "aga"
)

// StackDeployer will deploy an AGA resource stack into AWS.
type StackDeployer interface {
	// Deploy an AGA resource stack.
	Deploy(ctx context.Context, stack core.Stack, metricsCollector lbcmetrics.MetricCollector, controllerName string) error

	// GetAcceleratorManager method to expose accelerator manager for cleanup operations
	GetAcceleratorManager() AcceleratorManager
}

// NewDefaultStackDeployer constructs new defaultStackDeployer for AGA resources.
func NewDefaultStackDeployer(cloud services.Cloud, config config.ControllerConfig, trackingProvider tracking.Provider,
	logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, controllerName string) *defaultStackDeployer {

	// Create actual managers
	agaTaggingManager := NewDefaultTaggingManager(cloud.GlobalAccelerator(), cloud.RGT(), logger)
	acceleratorManager := NewDefaultAcceleratorManager(cloud.GlobalAccelerator(), trackingProvider, agaTaggingManager, config.ExternalManagedTags, logger)
	// TODO: Create other managers when they are implemented
	// listenerManager := NewDefaultListenerManager(cloud.GlobalAccelerator(), trackingProvider, agaTaggingManager, config.ExternalManagedTags, logger)
	// endpointGroupManager := NewDefaultEndpointGroupManager(cloud.GlobalAccelerator(), trackingProvider, agaTaggingManager, config.ExternalManagedTags, logger)
	// endpointManager := NewDefaultEndpointManager(cloud.GlobalAccelerator(), logger)

	return &defaultStackDeployer{
		cloud:              cloud,
		controllerConfig:   config,
		trackingProvider:   trackingProvider,
		featureGates:       config.FeatureGates,
		logger:             logger,
		metricsCollector:   metricsCollector,
		controllerName:     controllerName,
		agaTaggingManager:  agaTaggingManager,
		acceleratorManager: acceleratorManager,
		// TODO: Set other managers when implemented
		// listenerManager:      listenerManager,
		// endpointGroupManager: endpointGroupManager,
		// endpointManager:      endpointManager,
	}
}

var _ StackDeployer = &defaultStackDeployer{}

// defaultStackDeployer is the default implementation for AGA StackDeployer
type defaultStackDeployer struct {
	cloud            services.Cloud
	controllerConfig config.ControllerConfig
	trackingProvider tracking.Provider
	featureGates     config.FeatureGates
	logger           logr.Logger
	metricsCollector lbcmetrics.MetricCollector
	controllerName   string

	// Actual managers
	agaTaggingManager  TaggingManager
	acceleratorManager AcceleratorManager
	// TODO: Add other managers when implemented
	// listenerManager      ListenerManager
	// endpointGroupManager EndpointGroupManager
	// endpointManager      EndpointManager
}

type ResourceSynthesizer interface {
	Synthesize(ctx context.Context) error
	PostSynthesize(ctx context.Context) error
}

// Deploy an AGA resource stack.
// The deployment follows the proper dependency chain:
// Creation order: Accelerator -> Listeners -> EndpointGroups -> Endpoints
// Deletion order: Endpoints -> EndpointGroups -> Listeners -> Accelerator
func (d *defaultStackDeployer) Deploy(ctx context.Context, stack core.Stack, metricsCollector lbcmetrics.MetricCollector, controllerName string) error {
	var synthesizers []ResourceSynthesizer

	// Creation order: Accelerator first, then dependent resources
	synthesizers = append(synthesizers,
		NewAcceleratorSynthesizer(d.cloud.GlobalAccelerator(), d.trackingProvider, d.agaTaggingManager, d.acceleratorManager, d.logger, d.featureGates, stack),
		// TODO: Add other synthesizers when managers are implemented
		// NewListenerSynthesizer(d.cloud.GlobalAccelerator(), d.trackingProvider, d.agaTaggingManager, d.listenerManager, d.logger, d.featureGates, stack),
		// NewEndpointGroupSynthesizer(d.cloud.GlobalAccelerator(), d.trackingProvider, d.agaTaggingManager, d.endpointGroupManager, d.logger, d.featureGates, stack),
		// NewEndpointSynthesizer(d.cloud.GlobalAccelerator(), d.trackingProvider, d.endpointManager, d.logger, d.featureGates, stack),
	)

	// Execute Synthesize in creation order
	for _, synthesizer := range synthesizers {
		var err error
		// Get synthesizer type name for better context
		synthesizerType := fmt.Sprintf("%T", synthesizer)
		synthesizeFn := func() {
			err = synthesizer.Synthesize(ctx)
		}
		d.metricsCollector.ObserveControllerReconcileLatency(controllerName, synthesizerType, synthesizeFn)
		if err != nil {
			return ctrlerrors.NewErrorWithMetrics(controllerName, synthesizerType, err, d.metricsCollector)
		}
	}

	// Execute PostSynthesize in reverse order (deletion order)
	// This ensures proper cleanup: Endpoints -> EndpointGroups -> Listeners -> Accelerator
	for i := len(synthesizers) - 1; i >= 0; i-- {
		if err := synthesizers[i].PostSynthesize(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (d *defaultStackDeployer) GetAcceleratorManager() AcceleratorManager {
	return d.acceleratorManager
}
