package ec2

import (
	"context"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
)

var _ core.Resource = &SecurityGroup{}

// SecurityGroup represents a EC2 SecurityGroup.
type SecurityGroup struct {
	// resource id
	id string

	//  desired state of SecurityGroup
	Spec SecurityGroupSpec `json:"spec"`

	// observed state of SecurityGroup
	Status *SecurityGroupStatus `json:"status,omitempty"`
}

// NewSecurityGroup constructs new SecurityGroup resource.
func NewSecurityGroup(stack core.Stack, id string, spec SecurityGroupSpec) *SecurityGroup {
	sg := &SecurityGroup{
		id:     id,
		Spec:   spec,
		Status: nil,
	}
	stack.AddResource(sg)
	return sg
}

// Type returns resource's Type.
func (sg *SecurityGroup) Type() string {
	return "AWS::EC2::SecurityGroup"
}

// ID returns resource's ID within stack.
func (sg *SecurityGroup) ID() string {
	return sg.id
}

// SetStatus sets the SecurityGroup's status
func (sg *SecurityGroup) SetStatus(status SecurityGroupStatus) {
	sg.Status = &status
}

// GroupID returns a token for this SecurityGroup's groupID.
func (sg *SecurityGroup) GroupID() core.StringToken {
	return core.NewResourceFieldStringToken(sg, "status/groupID",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			sg := res.(*SecurityGroup)
			if sg.Status == nil {
				return "", errors.Errorf("SecurityGroup is not fulfilled yet: %v", sg.ID())
			}
			return sg.Status.GroupID, nil
		},
	)
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
	GroupID string `json:"groupID"`
}
