package networking

import (
	"context"
	"fmt"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// SecurityGroupResolver is responsible for resolving the frontend security groups from the names or IDs
type SecurityGroupResolver interface {
	// ResolveViaNameOrID resolves security groups from the security group names or the IDs
	ResolveViaNameOrID(ctx context.Context, sgNameOrIDs []string) ([]string, error)
	// ResolveViaSelector resolves security groups from a SecurityGroupSelector
	ResolveViaSelector(ctx context.Context, sgSelector *v1beta1.SecurityGroupSelector) ([]string, error)
}

// NewDefaultSecurityGroupResolver constructs new defaultSecurityGroupResolver.
func NewDefaultSecurityGroupResolver(ec2Client services.EC2, vpcID string, clusterName string) *defaultSecurityGroupResolver {
	return &defaultSecurityGroupResolver{
		ec2Client:   ec2Client,
		vpcID:       vpcID,
		clusterName: clusterName,
	}
}

var _ SecurityGroupResolver = &defaultSecurityGroupResolver{}

// default implementation for SecurityGroupResolver
type defaultSecurityGroupResolver struct {
	ec2Client   services.EC2
	vpcID       string
	clusterName string
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

func (r *defaultSecurityGroupResolver) ResolveViaSelector(ctx context.Context, selector *v1beta1.SecurityGroupSelector) ([]string, error) {
	var chosenSGs []*ec2sdk.SecurityGroup
	var err error
	var explanation string
	if selector.IDs != nil {
		req := &ec2sdk.DescribeSecurityGroupsInput{
			GroupIds: make([]*string, 0, len(selector.IDs)),
		}
		for _, groupID := range selector.IDs {
			id := string(groupID)
			req.GroupIds = append(req.GroupIds, &id)
		}
		chosenSGs, err = r.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(chosenSGs) != len(selector.IDs) {
			return nil, fmt.Errorf("couldn't find all security groups: IDs: %v, found: %v", selector.IDs, len(chosenSGs))
		}
	} else {
		req := &ec2sdk.DescribeSecurityGroupsInput{
			Filters: []*ec2sdk.Filter{
				{
					Name:   awssdk.String("vpc-id"),
					Values: awssdk.StringSlice([]string{r.vpcID}),
				},
			},
		}
		for key, values := range selector.Tags {
			req.Filters = append(req.Filters, &ec2sdk.Filter{
				Name:   awssdk.String("tag:" + key),
				Values: awssdk.StringSlice(values),
			})
		}

		allSGs, err := r.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		explanation = fmt.Sprintf("%d match VPC and tags", len(allSGs))
		var filteredSGs []*ec2sdk.SecurityGroup
		taggedOtherCluster := 0
		for _, sg := range allSGs {
			if r.checkSecurityGroupIsNotTaggedForOtherClusters(sg) {
				filteredSGs = append(filteredSGs, sg)
			} else {
				taggedOtherCluster += 1
			}
		}
		if taggedOtherCluster > 0 {
			explanation += fmt.Sprintf(", %d tagged for other cluster", taggedOtherCluster)
		}
		for _, sg := range filteredSGs {
			if r.checkSecurityGroupHasClusterTag(sg) {
				chosenSGs = append(chosenSGs, sg)
			}
		}
		if len(chosenSGs) == 0 {
			chosenSGs = filteredSGs
		}
	}
	if len(chosenSGs) == 0 {
		return nil, fmt.Errorf("unable to resolve at least one security group (%s)", explanation)
	}
	resolvedSGIDs := make([]string, 0, len(chosenSGs))
	for _, sg := range chosenSGs {
		resolvedSGIDs = append(resolvedSGIDs, awssdk.StringValue(sg.GroupId))
	}
	sort.Strings(resolvedSGIDs)
	return resolvedSGIDs, nil
}

// checkSecurityGroupHasClusterTag checks if the subnet is tagged for the current cluster
func (r *defaultSecurityGroupResolver) checkSecurityGroupHasClusterTag(securityGroup *ec2sdk.SecurityGroup) bool {
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", r.clusterName)
	for _, tag := range securityGroup.Tags {
		if clusterResourceTagKey == awssdk.StringValue(tag.Key) {
			return true
		}
	}
	return false
}

// checkSecurityGroupIsNotTaggedForOtherClusters checks whether the security group is tagged for the current cluster
// or it doesn't contain the cluster tag at all. If the security group contains a tag for other clusters, then
// this check returns false so that the security group is not used for the load balancer.
func (r *defaultSecurityGroupResolver) checkSecurityGroupIsNotTaggedForOtherClusters(sg *ec2sdk.SecurityGroup) bool {
	clusterResourceTagPrefix := "kubernetes.io/cluster"
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", r.clusterName)
	hasClusterResourceTagPrefix := false
	for _, tag := range sg.Tags {
		tagKey := awssdk.StringValue(tag.Key)
		if tagKey == clusterResourceTagKey {
			return true
		}
		if strings.HasPrefix(tagKey, clusterResourceTagPrefix) {
			// If the cluster tag is for a different cluster, keep track of it and exclude
			// the security group if no matching tag found for the current cluster.
			hasClusterResourceTagPrefix = true
		}
	}
	if hasClusterResourceTagPrefix {
		return false
	}
	return true
}
