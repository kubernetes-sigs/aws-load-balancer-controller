package targetgroupbinding

import (
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// TargetInfo contains information about a TargetGroup target.
type TargetInfo struct {
	// The target's description
	Target elbv2types.TargetDescription

	// The target's health information.
	// If absent, the target's health information is unknown.
	TargetHealth *elbv2types.TargetHealth
}

// GetIdentifier this should match backend.Endpoint
func (t *TargetInfo) GetIdentifier() string {
	return fmt.Sprintf("%s:%d", *t.Target.Id, *t.Target.Port)
}

// IsHealthy returns whether target is healthy.
func (t *TargetInfo) IsHealthy() bool {
	if t.TargetHealth == nil {
		return false
	}
	return elbv2types.TargetHealthStateEnumHealthy == t.TargetHealth.State
}

// IsNotRegistered returns whether target is not registered.
func (t *TargetInfo) IsNotRegistered() bool {
	if t.TargetHealth == nil {
		return false
	}
	return elbv2types.TargetHealthStateEnumUnused == t.TargetHealth.State &&
		elbv2types.TargetHealthReasonEnumNotRegistered == t.TargetHealth.Reason
}

// IsDraining returns whether target is in draining state.
func (t *TargetInfo) IsDraining() bool {
	if t.TargetHealth == nil {
		return false
	}
	return elbv2types.TargetHealthStateEnumDraining == t.TargetHealth.State ||
		elbv2types.TargetHealthStateEnumUnhealthyDraining == t.TargetHealth.State
}

// IsInitial returns whether target is in initial state.
func (t *TargetInfo) IsInitial() bool {
	if t.TargetHealth == nil {
		return false
	}
	return elbv2types.TargetHealthStateEnumInitial == t.TargetHealth.State
}

// UniqueIDForTargetDescription generates a unique ID to differentiate targets.
func UniqueIDForTargetDescription(target elbv2types.TargetDescription) string {
	return fmt.Sprintf("%v:%v", awssdk.ToString(target.Id), awssdk.ToInt32(target.Port))
}
