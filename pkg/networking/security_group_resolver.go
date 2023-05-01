package networking

import (
	"context"
	"strings"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// SecurityGroupResolver is responsible for resolving the frontend security groups from the names or IDs
type SecurityGroupResolver interface {
	// ResolveViaNameOrID resolves security groups from the security group names or the IDs
	ResolveViaNameOrID(ctx context.Context, sgNameOrIDs []string) ([]string, error)
}

// NewDefaultSecurityGroupResolver constructs new defaultSecurityGroupResolver.
func NewDefaultSecurityGroupResolver(ec2Client services.EC2, vpcID string) *defaultSecurityGroupResolver {
	return &defaultSecurityGroupResolver{
		ec2Client: ec2Client,
		vpcID:     vpcID,
	}
}

var _ SecurityGroupResolver = &defaultSecurityGroupResolver{}

// default implementation for SecurityGroupResolver
type defaultSecurityGroupResolver struct {
	ec2Client services.EC2
	vpcID     string
}

func (r *defaultSecurityGroupResolver) ResolveViaNameOrID(ctx context.Context, sgNameOrIDs []string) ([]string, error) {
	sgIDs, sgNames := r.splitIntoSgNameAndIDs(sgNameOrIDs)
	var resolvedSGs []*ec2sdk.SecurityGroup
	if len(sgIDs) > 0 {
		sgs, err := r.resolveViaGroupID(ctx, sgIDs)
		if err != nil {
			return nil, err
		}
		resolvedSGs = append(resolvedSGs, sgs...)
	}
	if len(sgNames) > 0 {
		sgs, err := r.resolveViaGroupName(ctx, sgNames)
		if err != nil {
			return nil, err
		}
		resolvedSGs = append(resolvedSGs, sgs...)
	}
	resolvedSGIDs := make([]string, 0, len(resolvedSGs))
	for _, sg := range resolvedSGs {
		resolvedSGIDs = append(resolvedSGIDs, awssdk.StringValue(sg.GroupId))
	}
	if len(resolvedSGIDs) != len(sgNameOrIDs) {
		return nil, errors.Errorf("couldn't find all securityGroups, nameOrIDs: %v, found: %v", sgNameOrIDs, resolvedSGIDs)
	}
	return resolvedSGIDs, nil
}

func (r *defaultSecurityGroupResolver) resolveViaGroupID(ctx context.Context, sgIDs []string) ([]*ec2sdk.SecurityGroup, error) {
	req := &ec2sdk.DescribeSecurityGroupsInput{
		GroupIds: awssdk.StringSlice(sgIDs),
	}
	sgs, err := r.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	return sgs, nil
}

func (r *defaultSecurityGroupResolver) resolveViaGroupName(ctx context.Context, sgNames []string) ([]*ec2sdk.SecurityGroup, error) {
	req := &ec2sdk.DescribeSecurityGroupsInput{
		Filters: []*ec2sdk.Filter{
			{
				Name:   awssdk.String("tag:Name"),
				Values: awssdk.StringSlice(sgNames),
			},
			{
				Name:   awssdk.String("vpc-id"),
				Values: awssdk.StringSlice([]string{r.vpcID}),
			},
		},
	}
	sgs, err := r.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	return sgs, nil
}

func (r *defaultSecurityGroupResolver) splitIntoSgNameAndIDs(sgNameOrIDs []string) ([]string, []string) {
	var sgIDs []string
	var sgNames []string
	for _, nameOrID := range sgNameOrIDs {
		if strings.HasPrefix(nameOrID, "sg-") {
			sgIDs = append(sgIDs, nameOrID)
		} else {
			sgNames = append(sgNames, nameOrID)
		}
	}
	return sgIDs, sgNames
}
