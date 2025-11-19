package aga

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ModelBuilder is responsible for building model stack for a GlobalAccelerator.
type ModelBuilder interface {
	// Build model stack for a GlobalAccelerator.
	Build(ctx context.Context, ga *agaapi.GlobalAccelerator) (core.Stack, *agamodel.Accelerator, error)
}

// NewDefaultModelBuilder constructs new defaultModelBuilder.
func NewDefaultModelBuilder(k8sClient client.Client, eventRecorder record.EventRecorder,
	trackingProvider tracking.Provider, featureGates config.FeatureGates,
	clusterName string, clusterRegion string, defaultTags map[string]string, externalManagedTags []string, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector) *defaultModelBuilder {

	return &defaultModelBuilder{
		k8sClient:           k8sClient,
		eventRecorder:       eventRecorder,
		trackingProvider:    trackingProvider,
		featureGates:        featureGates,
		clusterName:         clusterName,
		clusterRegion:       clusterRegion,
		defaultTags:         defaultTags,
		externalManagedTags: externalManagedTags,
		logger:              logger,
		metricsCollector:    metricsCollector,
	}
}

var _ ModelBuilder = &defaultModelBuilder{}

// default implementation for ModelBuilder
type defaultModelBuilder struct {
	k8sClient           client.Client
	eventRecorder       record.EventRecorder
	trackingProvider    tracking.Provider
	featureGates        config.FeatureGates
	clusterName         string
	clusterRegion       string
	defaultTags         map[string]string
	externalManagedTags []string
	logger              logr.Logger
	metricsCollector    lbcmetrics.MetricCollector
}

// Build model stack for a GlobalAccelerator.
func (b *defaultModelBuilder) Build(ctx context.Context, ga *agaapi.GlobalAccelerator) (core.Stack, *agamodel.Accelerator, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(ga)))

	// Create fresh builder instances for each reconciliation
	acceleratorBuilder := NewAcceleratorBuilder(b.trackingProvider, b.clusterName, b.clusterRegion, b.defaultTags, b.externalManagedTags, b.featureGates.Enabled(config.EnableDefaultTagsLowPriority))
	// TODO
	// endpointGroupBuilder := NewEndpointGroupBuilder()
	// endpointBuilder := NewEndpointBuilder()

	// Build Accelerator
	accelerator, err := acceleratorBuilder.Build(ctx, stack, ga)
	if err != nil {
		return nil, nil, err
	}

	// Build Listeners if specified
	var listeners []*agamodel.Listener
	if ga.Spec.Listeners != nil {
		// Create builder for listeners and endpoints
		listenerBuilder := NewListenerBuilder()
		listeners, err = listenerBuilder.Build(ctx, stack, accelerator, *ga.Spec.Listeners)
		if err != nil {
			return nil, nil, err
		}
	}

	b.logger.V(1).Info("Listeners built", "listeners", listeners)
	// TODO: Add other resource builders
	// endpointGroups, err := endpointGroupBuilder.Build(ctx, stack, listeners, ga.Spec.Listeners)
	// endpoints, err := endpointBuilder.Build(ctx, stack, endpointGroups, ga.Spec.Listeners)

	return stack, accelerator, nil
}
