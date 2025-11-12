package aga

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

// ListenerManager is responsible for managing AWS Global Accelerator listeners.
type ListenerManager interface {
	// Create creates a listener.
	Create(ctx context.Context, resListener *agamodel.Listener) (agamodel.ListenerStatus, error)

	// Update updates a listener.
	Update(ctx context.Context, resListener *agamodel.Listener, sdkListener *ListenerResource) (agamodel.ListenerStatus, error)

	// Delete deletes a listener.
	Delete(ctx context.Context, listenerARN string) error
}

// NewDefaultListenerManager constructs new defaultListenerManager.
func NewDefaultListenerManager(gaService services.GlobalAccelerator, logger logr.Logger) *defaultListenerManager {
	return &defaultListenerManager{
		gaService: gaService,
		logger:    logger,
	}
}

var _ ListenerManager = &defaultListenerManager{}

// defaultListenerManager is the default implementation for ListenerManager.
type defaultListenerManager struct {
	gaService services.GlobalAccelerator
	logger    logr.Logger
}

// convertPortRangesToSDK converts model port ranges to SDK port ranges
func convertPortRangesToSDK(modelPortRanges []agamodel.PortRange) []agatypes.PortRange {
	sdkPortRanges := make([]agatypes.PortRange, 0, len(modelPortRanges))
	for _, pr := range modelPortRanges {
		sdkPortRanges = append(sdkPortRanges, agatypes.PortRange{
			FromPort: aws.Int32(pr.FromPort),
			ToPort:   aws.Int32(pr.ToPort),
		})
	}
	return sdkPortRanges
}

func (m *defaultListenerManager) buildSDKCreateListenerInput(_ context.Context, resListener *agamodel.Listener) (*globalaccelerator.CreateListenerInput, error) {
	acceleratorARN, err := resListener.Spec.AcceleratorARN.Resolve(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve accelerator ARN")
	}

	// Convert port ranges to AWS SDK format
	portRanges := convertPortRangesToSDK(resListener.Spec.PortRanges)

	// Build create input
	createInput := &globalaccelerator.CreateListenerInput{
		AcceleratorArn: aws.String(acceleratorARN),
		Protocol:       agatypes.Protocol(resListener.Spec.Protocol),
		PortRanges:     portRanges,
	}

	// Add client affinity if specified
	if resListener.Spec.ClientAffinity != "" {
		createInput.ClientAffinity = agatypes.ClientAffinity(resListener.Spec.ClientAffinity)
	}

	return createInput, nil
}

func (m *defaultListenerManager) Create(ctx context.Context, resListener *agamodel.Listener) (agamodel.ListenerStatus, error) {
	// Build create input
	createInput, err := m.buildSDKCreateListenerInput(ctx, resListener)
	if err != nil {
		return agamodel.ListenerStatus{}, err
	}

	// Create listener
	m.logger.Info("Creating listener",
		"stackID", resListener.Stack().StackID(),
		"resourceID", resListener.ID())
	createOutput, err := m.gaService.CreateListenerWithContext(ctx, createInput)
	if err != nil {
		return agamodel.ListenerStatus{}, fmt.Errorf("failed to create listener: %w", err)
	}

	listener := createOutput.Listener
	m.logger.Info("Successfully created listener",
		"stackID", resListener.Stack().StackID(),
		"resourceID", resListener.ID(),
		"listenerARN", *listener.ListenerArn)

	return agamodel.ListenerStatus{
		ListenerARN: *listener.ListenerArn,
	}, nil
}

func (m *defaultListenerManager) buildSDKUpdateListenerInput(ctx context.Context, resListener *agamodel.Listener, sdkListener *ListenerResource) (*globalaccelerator.UpdateListenerInput, error) {
	// Convert port ranges to AWS SDK format
	portRanges := convertPortRangesToSDK(resListener.Spec.PortRanges)

	// Build update input
	updateInput := &globalaccelerator.UpdateListenerInput{
		ListenerArn: sdkListener.Listener.ListenerArn,
		Protocol:    agatypes.Protocol(resListener.Spec.Protocol),
		PortRanges:  portRanges,
	}

	// Add client affinity if specified
	if resListener.Spec.ClientAffinity != "" {
		updateInput.ClientAffinity = agatypes.ClientAffinity(resListener.Spec.ClientAffinity)
	}

	return updateInput, nil
}

func (m *defaultListenerManager) Update(ctx context.Context, resListener *agamodel.Listener, sdkListener *ListenerResource) (agamodel.ListenerStatus, error) {
	// Check if the listener actually needs an update
	if !m.isSDKListenerSettingsDrifted(resListener, sdkListener) {
		m.logger.Info("No drift detected in listener settings, skipping update",
			"stackID", resListener.Stack().StackID(),
			"resourceID", resListener.ID(),
			"listenerARN", *sdkListener.Listener.ListenerArn)
		return agamodel.ListenerStatus{
			ListenerARN: *sdkListener.Listener.ListenerArn,
		}, nil
	}

	m.logger.Info("Drift detected in listener settings, updating",
		"stackID", resListener.Stack().StackID(),
		"resourceID", resListener.ID(),
		"listenerARN", *sdkListener.Listener.ListenerArn)

	// Build update input
	updateInput, err := m.buildSDKUpdateListenerInput(ctx, resListener, sdkListener)
	if err != nil {
		return agamodel.ListenerStatus{}, err
	}

	// Update listener
	updateOutput, err := m.gaService.UpdateListenerWithContext(ctx, updateInput)
	if err != nil {
		return agamodel.ListenerStatus{}, fmt.Errorf("failed to update listener: %w", err)
	}
	updatedListener := updateOutput.Listener

	m.logger.Info("Successfully updated listener",
		"stackID", resListener.Stack().StackID(),
		"resourceID", resListener.ID(),
		"listenerARN", *updatedListener.ListenerArn)

	return agamodel.ListenerStatus{
		ListenerARN: *updatedListener.ListenerArn,
	}, nil
}

func (m *defaultListenerManager) Delete(ctx context.Context, listenerARN string) error {
	// TODO: This will be enhanced to check for and delete endpoint groups
	// before deleting the listener (when those features are implemented)

	m.logger.Info("Deleting listener", "listenerARN", listenerARN)

	deleteInput := &globalaccelerator.DeleteListenerInput{
		ListenerArn: aws.String(listenerARN),
	}

	if _, err := m.gaService.DeleteListenerWithContext(ctx, deleteInput); err != nil {
		// Check if it's a not found error - the listener might have already been deleted
		var apiErr *agatypes.ListenerNotFoundException
		if errors.As(err, &apiErr) {
			m.logger.Info("Listener already deleted", "listenerARN", listenerARN)
			return nil
		}
		return fmt.Errorf("failed to delete listener: %w", err)
	}

	m.logger.Info("Successfully deleted listener", "listenerARN", listenerARN)
	return nil
}

// isSDKListenerSettingsDrifted checks if the listener configuration has drifted from the desired state
func (m *defaultListenerManager) isSDKListenerSettingsDrifted(resListener *agamodel.Listener, sdkListener *ListenerResource) bool {
	// Check if protocol differs
	if string(resListener.Spec.Protocol) != string(sdkListener.Listener.Protocol) {
		return true
	}

	// Check if client affinity differs
	if string(resListener.Spec.ClientAffinity) != string(sdkListener.Listener.ClientAffinity) {
		return true
	}

	// Check if port ranges differ
	if !m.arePortRangesEqual(resListener.Spec.PortRanges, sdkListener.Listener.PortRanges) {
		return true
	}

	return false
}

// arePortRangesEqual compares port ranges from the resource model and SDK
func (m *defaultListenerManager) arePortRangesEqual(modelPortRanges []agamodel.PortRange, sdkPortRanges []agatypes.PortRange) bool {
	if len(modelPortRanges) != len(sdkPortRanges) {
		return false
	}

	// Since port ranges are unordered, we need to compare them as sets
	modelSet := sets.New[string]()
	for _, portRange := range modelPortRanges {
		key := fmt.Sprintf("%d-%d", portRange.FromPort, portRange.ToPort)
		modelSet.Insert(key)
	}

	sdkSet := sets.New[string]()
	for _, portRange := range sdkPortRanges {
		key := fmt.Sprintf("%d-%d", *portRange.FromPort, *portRange.ToPort)
		sdkSet.Insert(key)
	}

	return modelSet.Equal(sdkSet)
}
