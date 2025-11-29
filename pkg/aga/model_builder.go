package aga

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
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
	Build(ctx context.Context, ga *agaapi.GlobalAccelerator, loadedEndpoints []*LoadedEndpoint) (core.Stack, *agamodel.Accelerator, error)
}

// NewDefaultModelBuilder constructs new defaultModelBuilder.
func NewDefaultModelBuilder(k8sClient client.Client, eventRecorder record.EventRecorder,
	trackingProvider tracking.Provider, featureGates config.FeatureGates,
	clusterName string, clusterRegion string, defaultTags map[string]string, externalManagedTags []string,
	logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, elbv2Client services.ELBV2) *defaultModelBuilder {

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
		elbv2Client:         elbv2Client,
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
	elbv2Client         services.ELBV2
}

// Build model stack for a GlobalAccelerator.
func (b *defaultModelBuilder) Build(ctx context.Context, ga *agaapi.GlobalAccelerator, loadedEndpoints []*LoadedEndpoint) (core.Stack, *agamodel.Accelerator, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(ga)))

	// Create fresh builder instances for each reconciliation
	acceleratorBuilder := NewAcceleratorBuilder(b.trackingProvider, b.clusterName, b.clusterRegion, b.defaultTags, b.externalManagedTags, b.featureGates.Enabled(config.EnableDefaultTagsLowPriority))
	listenerBuilder := NewListenerBuilder(b.k8sClient, b.logger, b.elbv2Client)
	endpointGroupBuilder := NewEndpointGroupBuilder(b.clusterRegion, ga.Namespace, b.logger)

	// Build Accelerator
	accelerator, err := acceleratorBuilder.Build(ctx, stack, ga)
	if err != nil {
		return nil, nil, err
	}

	// Build Listeners if specified
	var listeners []*agamodel.Listener
	var processedListeners []agaapi.GlobalAcceleratorListener
	if ga.Spec.Listeners != nil {
		listeners, processedListeners, err = listenerBuilder.Build(ctx, stack, accelerator, *ga.Spec.Listeners, ga, loadedEndpoints)
		if err != nil {
			return nil, nil, err
		}

		// Build endpoint groups with loaded endpoints - using processedListeners to capture auto-discovery changes
		_, err := endpointGroupBuilder.Build(ctx, stack, listeners, processedListeners, loadedEndpoints)
		if err != nil {
			return nil, nil, err
		}
	}

	return stack, accelerator, nil
}
