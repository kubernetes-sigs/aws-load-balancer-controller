package targetgroupbinding

import (
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
)

// TargetInfo contains information about a TargetGroup target.
type TargetInfo struct {
	// The target's description
	Target elbv2sdk.TargetDescription

	// The target's health information.
	// If absent, the target's health information is unknown.
	TargetHealth *elbv2sdk.TargetHealth
}

// IsHealthy returns whether target is healthy.
func (t *TargetInfo) IsHealthy() bool {
	if t.TargetHealth == nil {
		return false
	}
	return awssdk.StringValue(t.TargetHealth.State) == elbv2sdk.TargetHealthStateEnumHealthy
}

// IsNotRegistered returns whether target is not registered.
func (t *TargetInfo) IsNotRegistered() bool {
	if t.TargetHealth == nil {
		return false
	}
	return awssdk.StringValue(t.TargetHealth.State) == elbv2sdk.TargetHealthStateEnumUnused &&
		awssdk.StringValue(t.TargetHealth.Reason) == elbv2sdk.TargetHealthReasonEnumTargetNotRegistered
}

// IsDraining returns whether target is in draining state.
func (t *TargetInfo) IsDraining() bool {
	if t.TargetHealth == nil {
		return false
	}
	return awssdk.StringValue(t.TargetHealth.State) == elbv2sdk.TargetHealthStateEnumDraining
}

// IsInitial returns whether target is in initial state.
func (t *TargetInfo) IsInitial() bool {
	if t.TargetHealth == nil {
		return false
	}
	return awssdk.StringValue(t.TargetHealth.State) == elbv2sdk.TargetHealthStateEnumInitial
}

// UniqueIDForTargetDescription generates a unique ID to differentiate targets.
func UniqueIDForTargetDescription(target elbv2sdk.TargetDescription) string {
	return fmt.Sprintf("%v:%v:%v", awssdk.StringValue(target.Id), awssdk.Int64Value(target.Port), awssdk.StringValue(target.AvailabilityZone))
}
