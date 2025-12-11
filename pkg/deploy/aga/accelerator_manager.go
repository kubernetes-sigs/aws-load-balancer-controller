package aga

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

// AcceleratorManager is responsible for managing AWS Global Accelerator accelerators.
type AcceleratorManager interface {
	// Create creates an accelerator.
	Create(ctx context.Context, resAccelerator *agamodel.Accelerator) (agamodel.AcceleratorStatus, error)

	// Update updates an accelerator.
	Update(ctx context.Context, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags) (agamodel.AcceleratorStatus, error)

	// Delete deletes an accelerator.
	Delete(ctx context.Context, sdkAccelerator AcceleratorWithTags) error
}

// NewDefaultAcceleratorManager constructs new defaultAcceleratorManager.
func NewDefaultAcceleratorManager(gaService services.GlobalAccelerator, trackingProvider tracking.Provider, taggingManager TaggingManager, listenerManager ListenerManager, externalManagedTags []string, logger logr.Logger) *defaultAcceleratorManager {
	return &defaultAcceleratorManager{
		gaService:           gaService,
		trackingProvider:    trackingProvider,
		taggingManager:      taggingManager,
		listenerManager:     listenerManager,
		externalManagedTags: externalManagedTags,
		logger:              logger,
	}
}

var _ AcceleratorManager = &defaultAcceleratorManager{}

// defaultAcceleratorManager is the default implementation for AcceleratorManager.
type defaultAcceleratorManager struct {
	gaService           services.GlobalAccelerator
	trackingProvider    tracking.Provider
	taggingManager      TaggingManager
	listenerManager     ListenerManager
	externalManagedTags []string
	logger              logr.Logger
}

func (m *defaultAcceleratorManager) buildSDKCreateAcceleratorInput(_ context.Context, resAccelerator *agamodel.Accelerator) *globalaccelerator.CreateAcceleratorInput {
	idempotencyToken := m.getIdempotencyToken(resAccelerator)
	// Build create input
	createInput := &globalaccelerator.CreateAcceleratorInput{
		Name:             aws.String(resAccelerator.Spec.Name),
		IpAddressType:    agatypes.IpAddressType(resAccelerator.Spec.IPAddressType),
		Enabled:          resAccelerator.Spec.Enabled,
		IdempotencyToken: aws.String(idempotencyToken),
	}

	// BYOIP feature: Set IP addresses if provided
	if len(resAccelerator.Spec.IpAddresses) > 0 {
		createInput.IpAddresses = resAccelerator.Spec.IpAddresses
	}

	// Add tags
	tags := m.trackingProvider.ResourceTags(resAccelerator.Stack(), resAccelerator, resAccelerator.Spec.Tags)
	createInput.Tags = m.taggingManager.ConvertTagsToSDKTags(tags)

	return createInput
}

func (m *defaultAcceleratorManager) Create(ctx context.Context, resAccelerator *agamodel.Accelerator) (agamodel.AcceleratorStatus, error) {

	// Build create input
	createInput := m.buildSDKCreateAcceleratorInput(ctx, resAccelerator)

	// Create accelerator
	m.logger.Info("Creating accelerator",
		"stackID", resAccelerator.Stack().StackID(),
		"resourceID", resAccelerator.ID())
	createOutput, err := m.gaService.CreateAcceleratorWithContext(ctx, createInput)
	if err != nil {
		return agamodel.AcceleratorStatus{}, fmt.Errorf("failed to create accelerator: %w", err)
	}

	accelerator := createOutput.Accelerator
	m.logger.Info("Successfully created accelerator",
		"stackID", resAccelerator.Stack().StackID(),
		"resourceID", resAccelerator.ID(),
		"acceleratorARN", *accelerator.AcceleratorArn)

	return m.buildAcceleratorStatus(accelerator), nil
}

func (m *defaultAcceleratorManager) buildSDKUpdateAcceleratorInput(ctx context.Context, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags) *globalaccelerator.UpdateAcceleratorInput {
	// Build update input
	updateInput := &globalaccelerator.UpdateAcceleratorInput{
		AcceleratorArn: sdkAccelerator.Accelerator.AcceleratorArn,
		Name:           aws.String(resAccelerator.Spec.Name),
		IpAddressType:  agatypes.IpAddressType(resAccelerator.Spec.IPAddressType),
		Enabled:        resAccelerator.Spec.Enabled,
	}
	// BYOIP is only supported during accelerator creation, not updates

	return updateInput
}

func (m *defaultAcceleratorManager) Update(ctx context.Context, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags) (agamodel.AcceleratorStatus, error) {

	if err := m.updateAcceleratorTags(ctx, resAccelerator, sdkAccelerator); err != nil {
		return agamodel.AcceleratorStatus{}, fmt.Errorf("failed to update accelerator tags: %w", err)
	}

	var updatedAccelerator *agatypes.Accelerator
	if !m.isSDKAcceleratorSettingsDrifted(resAccelerator, sdkAccelerator) {
		m.logger.Info("No drift detected in accelerator settings, skipping update",
			"stackID", resAccelerator.Stack().StackID(),
			"resourceID", resAccelerator.ID(),
			"acceleratorARN", *sdkAccelerator.Accelerator.AcceleratorArn)
		return m.buildAcceleratorStatus(sdkAccelerator.Accelerator), nil
	}
	m.logger.Info("Drift detected in accelerator settings, updating",
		"stackID", resAccelerator.Stack().StackID(),
		"resourceID", resAccelerator.ID(),
		"acceleratorARN", *sdkAccelerator.Accelerator.AcceleratorArn)

	// Build update input
	updateInput := m.buildSDKUpdateAcceleratorInput(ctx, resAccelerator, sdkAccelerator)

	// Update accelerator
	updateOutput, err := m.gaService.UpdateAcceleratorWithContext(ctx, updateInput)
	if err != nil {
		return agamodel.AcceleratorStatus{}, fmt.Errorf("failed to update accelerator: %w", err)
	}
	updatedAccelerator = updateOutput.Accelerator

	m.logger.Info("Successfully updated accelerator",
		"stackID", resAccelerator.Stack().StackID(),
		"resourceID", resAccelerator.ID(),
		"acceleratorARN", *updatedAccelerator.AcceleratorArn)

	return m.buildAcceleratorStatus(updatedAccelerator), nil
}

func (m *defaultAcceleratorManager) Delete(ctx context.Context, sdkAccelerator AcceleratorWithTags) error {
	acceleratorARN := awssdk.ToString(sdkAccelerator.Accelerator.AcceleratorArn)
	m.logger.Info("Deleting accelerator", "acceleratorARN", acceleratorARN)

	// Step 1: Try to disable the accelerator first if it's enabled
	if sdkAccelerator.Accelerator.Enabled == nil || awssdk.ToBool(sdkAccelerator.Accelerator.Enabled) == true {
		m.logger.Info("Disabling accelerator before deletion", "acceleratorARN", acceleratorARN)
		isAlreadyDeleted, err := m.disableAccelerator(ctx, acceleratorARN)
		if err != nil {
			return fmt.Errorf("failed to disable accelerator: %w", err)
		}
		if isAlreadyDeleted {
			return nil
		}
	}

	// Step 2: Delete all listeners associated with this accelerator
	// TODO: This will be enhanced to delete endpoint groups and endpoints
	// before deleting listeners (when those features are implemented)
	listeners, err := m.listListeners(ctx, acceleratorARN)
	if err != nil {
		var apiErr *agatypes.AcceleratorNotFoundException
		if errors.As(err, &apiErr) {
			m.logger.Info("Accelerator not found, assuming already deleted", "acceleratorARN", acceleratorARN)
			return nil
		}
		return fmt.Errorf("failed to list listeners for accelerator: %w", err)
	}

	for _, listener := range listeners {
		listenerARN := awssdk.ToString(listener.ListenerArn)
		m.logger.Info("Deleting listener for accelerator", "listenerARN", listenerARN, "acceleratorARN", acceleratorARN)

		if err := m.listenerManager.Delete(ctx, listenerARN); err != nil {
			return fmt.Errorf("failed to delete listener %s: %w", listenerARN, err)
		}
	}

	// Step 3: Delete the accelerator
	deleteInput := &globalaccelerator.DeleteAcceleratorInput{
		AcceleratorArn: aws.String(acceleratorARN),
	}

	if _, err := m.gaService.DeleteAcceleratorWithContext(ctx, deleteInput); err != nil {
		// Check if it's an AcceleratorNotDisabledException
		var notDisabledErr *agatypes.AcceleratorNotDisabledException
		if errors.As(err, &notDisabledErr) {
			// This happens if the accelerator is still in the process of being disabled
			return &AcceleratorNotDisabledError{
				Message: "Accelerator is not fully disabled yet",
			}
		}

		// Check if accelerator was already deleted
		var apiErr *agatypes.AcceleratorNotFoundException
		if errors.As(err, &apiErr) {
			m.logger.Info("Accelerator already deleted", "acceleratorARN", acceleratorARN)
			return nil
		}

		return fmt.Errorf("failed to delete accelerator: %w", err)
	}

	m.logger.Info("Successfully deleted accelerator", "acceleratorARN", acceleratorARN)
	return nil
}

func (m *defaultAcceleratorManager) disableAccelerator(ctx context.Context, acceleratorARN string) (bool, error) {
	// First, describe the accelerator to check if it's already disabled
	describeInput := &globalaccelerator.DescribeAcceleratorInput{
		AcceleratorArn: aws.String(acceleratorARN),
	}

	describeOutput, err := m.gaService.DescribeAcceleratorWithContext(ctx, describeInput)
	if err != nil {
		var notFoundErr *agatypes.AcceleratorNotFoundException
		if errors.As(err, &notFoundErr) {
			// Accelerator doesn't exist anymore, nothing to do
			m.logger.Info("Accelerator not found, assuming already deleted", "acceleratorARN", acceleratorARN)
			return true, nil
		}
		return false, fmt.Errorf("failed to describe accelerator: %w", err)
	}

	if awssdk.ToBool(describeOutput.Accelerator.Enabled) == false {
		m.logger.Info("Accelerator is already disabled, proceeding with deletion", "acceleratorARN", acceleratorARN)
		return false, nil
	}
	updateInput := &globalaccelerator.UpdateAcceleratorInput{
		AcceleratorArn: aws.String(acceleratorARN),
		Enabled:        aws.Bool(false),
	}

	if _, err := m.gaService.UpdateAcceleratorWithContext(ctx, updateInput); err != nil {
		return false, fmt.Errorf("failed to disable accelerator: %w", err)
	}

	return false, nil
}

func (m *defaultAcceleratorManager) updateAcceleratorTags(ctx context.Context, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags) error {
	desiredTags := m.trackingProvider.ResourceTags(resAccelerator.Stack(), resAccelerator, resAccelerator.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, *sdkAccelerator.Accelerator.AcceleratorArn, desiredTags,
		WithCurrentTags(sdkAccelerator.Tags),
		WithIgnoredTagKeys(m.externalManagedTags))

}

func (m *defaultAcceleratorManager) isSDKAcceleratorSettingsDrifted(resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags) bool {
	// Check if name differs
	if resAccelerator.Spec.Name != *sdkAccelerator.Accelerator.Name {
		return true
	}

	// Check if IP address type differs
	if string(resAccelerator.Spec.IPAddressType) != string(sdkAccelerator.Accelerator.IpAddressType) {
		return true
	}

	// Check if enabled state differs
	if *resAccelerator.Spec.Enabled != *sdkAccelerator.Accelerator.Enabled {
		return true
	}

	// Check if user attempts to change IP addresses (BYOIP only supported at creation)
	if len(resAccelerator.Spec.IpAddresses) > 0 && !m.areIPAddressesEqual(resAccelerator.Spec.IpAddresses, sdkAccelerator.Accelerator.IpSets) {
		m.logger.Info("IP addresses cannot be updated after accelerator creation, ignoring IP address changes")
	}

	return false
}

func (m *defaultAcceleratorManager) areIPAddressesEqual(desiredIPs []string, actualIPSets []agatypes.IpSet) bool {

	// IPv6 BYOIP is not supported at this time
	return m.areIPv4AddressesEqual(desiredIPs, actualIPSets)
}

// areIPv4AddressesEqual compares desired IPv4 addresses with actual IP sets from AWS
func (m *defaultAcceleratorManager) areIPv4AddressesEqual(desiredIPs []string, actualIPSets []agatypes.IpSet) bool {
	actualIPv4s := extractIPv4Addresses(actualIPSets)
	if len(desiredIPs) != len(actualIPv4s) {
		return false
	}

	slices.Sort(desiredIPs)
	slices.Sort(actualIPv4s)
	return slices.Equal(desiredIPs, actualIPv4s)
}

// extractIPv4Addresses extracts IPv4 addresses from IPSets
func extractIPv4Addresses(ipSets []agatypes.IpSet) []string {
	ips := make([]string, 0)
	for _, ipSet := range ipSets {
		if ipSet.IpAddressFamily == "IPv4" {
			ips = append(ips, ipSet.IpAddresses...)
		}
	}
	return ips
}

func (m *defaultAcceleratorManager) getIdempotencyToken(resAccelerator *agamodel.Accelerator) string {
	// Use the CRD's UID as the idempotency token as its unique
	return resAccelerator.GetCRDUID()
}

// listListeners lists all listeners for a given accelerator
func (m *defaultAcceleratorManager) listListeners(ctx context.Context, acceleratorARN string) ([]agatypes.Listener, error) {
	listInput := &globalaccelerator.ListListenersInput{
		AcceleratorArn: aws.String(acceleratorARN),
	}

	return m.gaService.ListListenersAsList(ctx, listInput)
}

func (m *defaultAcceleratorManager) buildAcceleratorStatus(accelerator *agatypes.Accelerator) agamodel.AcceleratorStatus {
	status := agamodel.AcceleratorStatus{
		AcceleratorARN: *accelerator.AcceleratorArn,
		DNSName:        *accelerator.DnsName,
		Status:         string(accelerator.Status),
		IPSets:         []agamodel.IPSet{},
	}

	if accelerator.DualStackDnsName != nil {
		status.DualStackDNSName = *accelerator.DualStackDnsName
	}

	// Convert IP sets
	for _, ipSet := range accelerator.IpSets {
		agaIPSet := agamodel.IPSet{
			IpAddressFamily: string(ipSet.IpAddressFamily),
			IpAddresses:     ipSet.IpAddresses,
		}
		status.IPSets = append(status.IPSets, agaIPSet)
	}

	return status
}
