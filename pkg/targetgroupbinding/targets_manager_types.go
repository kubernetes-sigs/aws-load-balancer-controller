package targetgroupbinding

import (
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
)

// TargetInfo contains information about a TargetGroup target
type TargetInfo struct {
	// The target's description
	Target elbv2sdk.TargetDescription

	// The target's health information.
	// If absent, the target's health information is unknown.
	TargetHealth *elbv2sdk.TargetHealth
}

// IsOngoing returns whether target is in ongoing status
func (t *TargetInfo) IsOngoing() bool {
	if t.TargetHealth == nil {
		return true
	}
	switch awssdk.StringValue(t.TargetHealth.State) {
	case elbv2sdk.TargetHealthStateEnumInitial, elbv2sdk.TargetHealthStateEnumDraining:
		return true
	}
	return false
}

// IsNotRegistered returns whether target is not registered
func (t *TargetInfo) IsNotRegistered() bool {
	if t.TargetHealth == nil {
		return false
	}
	return awssdk.StringValue(t.TargetHealth.State) == elbv2sdk.TargetHealthStateEnumUnused &&
		awssdk.StringValue(t.TargetHealth.Reason) == elbv2sdk.TargetHealthReasonEnumTargetNotRegistered
}

// UniqueIDForTargetDescription generates a unique ID to differentiate targets.
func UniqueIDForTargetDescription(target elbv2sdk.TargetDescription) string {
	return fmt.Sprintf("%v:%v", awssdk.StringValue(target.Id), awssdk.Int64Value(target.Port))
}
