package aga

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Condition type constants
	ConditionTypeReady                = "Ready"
	ConditionTypeAcceleratorDisabling = "AcceleratorDisabling"

	// Reason constants
	ReasonAcceleratorReady        = "AcceleratorReady"
	ReasonAcceleratorProvisioning = "AcceleratorProvisioning"
	ReasonAcceleratorDisabling    = "AcceleratorDisabling"
	ReasonAcceleratorDeleting     = "AcceleratorDeleting"

	// Status constants
	StatusDeployed   = "DEPLOYED"
	StatusInProgress = "IN_PROGRESS"
	StatusDeleting   = "DELETING"
)

// StatusUpdater handles GlobalAccelerator resource status updates
type StatusUpdater interface {
	// UpdateStatusSuccess updates the GlobalAccelerator status after successful deployment
	UpdateStatusSuccess(ctx context.Context, ga *v1beta1.GlobalAccelerator, accelerator *agamodel.Accelerator) (bool, error)

	// UpdateStatusFailure updates the GlobalAccelerator status when deployment fails
	UpdateStatusFailure(ctx context.Context, ga *v1beta1.GlobalAccelerator, reason, message string) error

	// UpdateStatusDeletion updates the GlobalAccelerator status during deletion process
	UpdateStatusDeletion(ctx context.Context, ga *v1beta1.GlobalAccelerator) error
}

// NewStatusUpdater creates a new StatusUpdater
func NewStatusUpdater(k8sClient client.Client, logger logr.Logger) StatusUpdater {
	return &defaultStatusUpdater{
		k8sClient: k8sClient,
		logger:    logger.WithName("aga-status-updater"),
	}
}

// defaultStatusUpdater is the default implementation of StatusUpdater
type defaultStatusUpdater struct {
	k8sClient client.Client
	logger    logr.Logger
}

// UpdateStatusSuccess updates the GlobalAccelerator status after successful deployment
// Returns true if requeue is needed for status polling
func (u *defaultStatusUpdater) UpdateStatusSuccess(ctx context.Context, ga *v1beta1.GlobalAccelerator,
	accelerator *agamodel.Accelerator) (bool, error) {

	// Accelerator status should always be set after deployment, if it's not, prevent NPE
	if accelerator.Status == nil {
		u.logger.Info("Unable to update GlobalAccelerator Status due to null accelerator status",
			"globalAccelerator", k8s.NamespacedName(ga))
		return false, nil
	}

	gaOld := ga.DeepCopy()
	var needPatch bool
	var requeueNeeded bool

	// Check if accelerator is fully deployed
	isDeployed := u.isAcceleratorDeployed(*accelerator.Status)

	// Update observed generation
	if ga.Status.ObservedGeneration == nil || *ga.Status.ObservedGeneration != ga.Generation {
		ga.Status.ObservedGeneration = &ga.Generation
		needPatch = true
	}

	// Update accelerator ARN
	if ga.Status.AcceleratorARN == nil || *ga.Status.AcceleratorARN != accelerator.Status.AcceleratorARN {
		ga.Status.AcceleratorARN = &accelerator.Status.AcceleratorARN
		needPatch = true
	}

	// Update DNS name
	if ga.Status.DNSName == nil || *ga.Status.DNSName != accelerator.Status.DNSName {
		ga.Status.DNSName = &accelerator.Status.DNSName
		needPatch = true
	}

	// Update dual stack DNS name
	if accelerator.Status.DualStackDNSName != "" {
		if ga.Status.DualStackDnsName == nil || *ga.Status.DualStackDnsName != accelerator.Status.DualStackDNSName {
			ga.Status.DualStackDnsName = &accelerator.Status.DualStackDNSName
			needPatch = true
		}
	} else if ga.Status.DualStackDnsName != nil {
		// Clear the field when DualStackDNSName is no longer available
		ga.Status.DualStackDnsName = nil
		needPatch = true
	}

	// Update IP sets
	if len(accelerator.Status.IPSets) > 0 {
		newIPSets := make([]v1beta1.IPSet, len(accelerator.Status.IPSets))
		for i, ipSet := range accelerator.Status.IPSets {
			newIPSets[i] = v1beta1.IPSet{
				IpAddresses:     &ipSet.IpAddresses,
				IpAddressFamily: &ipSet.IpAddressFamily,
			}
		}
		if !u.areIPSetsEqual(ga.Status.IPSets, newIPSets) {
			ga.Status.IPSets = newIPSets
			needPatch = true
		}
	}

	// Update status
	if ga.Status.Status == nil || *ga.Status.Status != accelerator.Status.Status {
		ga.Status.Status = &accelerator.Status.Status
		needPatch = true
	}

	// Update conditions based on deployment status
	var readyCondition metav1.Condition
	if isDeployed {
		readyCondition = metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             ReasonAcceleratorReady,
			Message:            "GlobalAccelerator is ready and available",
		}
	} else {
		// Set Ready to Unknown while accelerator is provisioning
		readyCondition = metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionUnknown,
			LastTransitionTime: metav1.Now(),
			Reason:             ReasonAcceleratorProvisioning,
			Message:            "GlobalAccelerator is being provisioned",
		}
		requeueNeeded = true
	}

	conditionUpdated := u.updateCondition(&ga.Status.Conditions, readyCondition)
	if conditionUpdated {
		needPatch = true
	}

	// Skip status update if observed generation already matches and nothing else changed
	if ga.Status.ObservedGeneration != nil && *ga.Status.ObservedGeneration == ga.Generation && !needPatch {
		u.logger.V(1).Info("Skipping status update - no changes needed", "globalAccelerator", k8s.NamespacedName(ga))
		return requeueNeeded, nil
	}

	if needPatch {
		if err := u.k8sClient.Status().Patch(ctx, ga, client.MergeFrom(gaOld)); err != nil {
			return requeueNeeded, errors.Wrapf(err, "failed to update GlobalAccelerator status: %v", k8s.NamespacedName(ga))
		}
		u.logger.Info("Successfully updated GlobalAccelerator status", "globalAccelerator", k8s.NamespacedName(ga))
	}

	return requeueNeeded, nil
}

// UpdateStatusFailure updates the GlobalAccelerator status when deployment fails
func (u *defaultStatusUpdater) UpdateStatusFailure(ctx context.Context, ga *v1beta1.GlobalAccelerator,
	reason, message string) error {

	gaOld := ga.DeepCopy()
	var needPatch bool

	// Update observed generation
	if ga.Status.ObservedGeneration == nil || *ga.Status.ObservedGeneration != ga.Generation {
		ga.Status.ObservedGeneration = &ga.Generation
		needPatch = true
	}

	// Set Ready condition to False with failure reason
	failureCondition := metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	conditionUpdated := u.updateCondition(&ga.Status.Conditions, failureCondition)
	if conditionUpdated {
		needPatch = true
	}

	// Skip status update if observed generation already matches and nothing else changed
	if ga.Status.ObservedGeneration != nil && *ga.Status.ObservedGeneration == ga.Generation && !needPatch {
		u.logger.V(1).Info("Skipping status update - no changes needed", "globalAccelerator", k8s.NamespacedName(ga))
		return nil
	}

	if needPatch {
		if err := u.k8sClient.Status().Patch(ctx, ga, client.MergeFrom(gaOld)); err != nil {
			return errors.Wrapf(err, "failed to update GlobalAccelerator status: %v", k8s.NamespacedName(ga))
		}
		u.logger.Info("Successfully updated GlobalAccelerator status with failure",
			"globalAccelerator", k8s.NamespacedName(ga),
			"reason", reason)
	}

	return nil
}

// UpdateStatusDeletion updates the GlobalAccelerator status during deletion process
func (u *defaultStatusUpdater) UpdateStatusDeletion(ctx context.Context, ga *v1beta1.GlobalAccelerator) error {
	gaOld := ga.DeepCopy()
	var needPatch bool

	// Update observed generation
	if ga.Status.ObservedGeneration == nil || *ga.Status.ObservedGeneration != ga.Generation {
		ga.Status.ObservedGeneration = &ga.Generation
		needPatch = true
	}

	// Set status to "Deleting" to indicate it's in the process of being deleted
	if ga.Status.Status == nil || *ga.Status.Status != StatusDeleting {
		deletingStatus := StatusDeleting
		ga.Status.Status = &deletingStatus
		needPatch = true
	}

	// Add a condition to indicate we're waiting for the accelerator to be disabled
	waitingCondition := metav1.Condition{
		Type:               ConditionTypeAcceleratorDisabling,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonAcceleratorDisabling,
		Message:            "Waiting for accelerator to be disabled before deletion",
	}

	// Set Ready condition to False during deletion
	readyCondition := metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonAcceleratorDeleting,
		Message:            "GlobalAccelerator is being deleted",
	}

	// Update both conditions
	conditionUpdated1 := u.updateCondition(&ga.Status.Conditions, waitingCondition)
	conditionUpdated2 := u.updateCondition(&ga.Status.Conditions, readyCondition)
	if conditionUpdated1 || conditionUpdated2 {
		needPatch = true
	}

	// Skip status update if nothing changed
	if !needPatch {
		return nil
	}

	if err := u.k8sClient.Status().Patch(ctx, ga, client.MergeFrom(gaOld)); err != nil {
		return errors.Wrapf(err, "failed to update GlobalAccelerator status: %v", k8s.NamespacedName(ga))
	}

	u.logger.Info("Updated GlobalAccelerator status for deletion",
		"globalAccelerator", k8s.NamespacedName(ga))

	return nil
}

// Helper methods

// isAcceleratorDeployed checks if the accelerator is fully deployed and ready
func (u *defaultStatusUpdater) isAcceleratorDeployed(acceleratorStatus agamodel.AcceleratorStatus) bool {
	// Check if the accelerator status indicates it's deployed
	// GlobalAccelerator status can be: IN_PROGRESS or DEPLOYED
	return acceleratorStatus.Status == StatusDeployed
}

// updateCondition updates or adds a condition to the conditions slice
func (u *defaultStatusUpdater) updateCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) bool {
	if conditions == nil {
		*conditions = []metav1.Condition{newCondition}
		return true
	}

	for i, condition := range *conditions {
		if condition.Type == newCondition.Type {
			if condition.Status != newCondition.Status ||
				condition.Reason != newCondition.Reason ||
				condition.Message != newCondition.Message {
				(*conditions)[i] = newCondition
				return true
			}
			return false
		}
	}

	// Condition not found, add it
	*conditions = append(*conditions, newCondition)
	return true
}

// areIPSetsEqual compares two slices of IPSets for equality
func (u *defaultStatusUpdater) areIPSetsEqual(existing []v1beta1.IPSet, new []v1beta1.IPSet) bool {
	return reflect.DeepEqual(existing, new)
}
