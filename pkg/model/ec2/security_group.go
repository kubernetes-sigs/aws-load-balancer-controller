package ec2

import (
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
)

var _ core.Resource = &SecurityGroup{}

// SecurityGroup represents a EC2 SecurityGroup.
type SecurityGroup struct {
	// resource id
	id string

	//  desired state of SecurityGroup
	spec SecurityGroupSpec

	// observed state of SecurityGroup
	status *SecurityGroupStatus
}

// NewSecurityGroup constructs new SecurityGroup resource.
func NewSecurityGroup(stack core.Stack, id string, spec SecurityGroupSpec) *SecurityGroup {
	sg := &SecurityGroup{
		id:     id,
		spec:   spec,
		status: nil,
	}
	stack.AddResource(sg)
	return sg
}

// ID returns resource's ID within stack.
func (sg *SecurityGroup) ID() string {
	return sg.id
}

// GroupID returns a token for this SecurityGroup's groupID.
func (sg *SecurityGroup) GroupID() core.StringToken {
	// TODO
	return nil
}

// SecurityGroupSpec defines the desired state of SecurityGroup
type SecurityGroupSpec struct {
	// The name of the security group.
	GroupName string `json:"groupName"`

	// A description for the security group.
	// +optional
	Description *string `json:"description,omitempty"`

	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SecurityGroupStatus defines the observed state of SecurityGroup
type SecurityGroupStatus struct {
	// The ID of the security group.
	// +optional
	GroupID *string `json:"groupID,omitempty"`
}
