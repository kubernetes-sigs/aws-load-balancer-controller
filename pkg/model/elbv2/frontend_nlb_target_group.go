package elbv2

import "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"

var _ core.Resource = &FrontendNlbTargetGroupDesiredState{}

const (
	FrontNLBResourceId = "FrontendNLBTargetGroup"
)

// FrontendNlbTargetGroupState represents the state of a single ALB Target Type target group with its ALB target
type FrontendNlbTargetGroupState struct {
	Name       string
	ARN        core.StringToken
	Port       int32
	TargetARN  core.StringToken
	TargetPort int32
}

// FrontendNlbTargetGroupDesiredState maintains a mapping of target groups targeting ALB
type FrontendNlbTargetGroupDesiredState struct {
	core.ResourceMeta `json:"-"`

	// Maps target group name -> The FE NLB configuration.
	TargetGroups map[string]*FrontendNlbTargetGroupState
}

func NewFrontendNlbTargetGroupDesiredState(stack core.Stack, stateConfig map[string]*FrontendNlbTargetGroupState) *FrontendNlbTargetGroupDesiredState {
	desiredState := &FrontendNlbTargetGroupDesiredState{
		ResourceMeta: core.NewResourceMeta(stack, FrontNLBResourceId, FrontNLBResourceId),
		TargetGroups: stateConfig,
	}
	stack.AddResource(desiredState)
	return desiredState
}
