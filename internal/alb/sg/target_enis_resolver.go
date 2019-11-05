package sg

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/utils"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

// the maximum number of filters in a single describeNetworkInterfaces call.
const EC2DescribeNetworkInterfacesFilterLimit = 200

type ENIInfo struct {
	eni         *ec2.NetworkInterface
	instanceENI *ec2.InstanceNetworkInterface
}

func NewENIInfoViaENI(eni *ec2.NetworkInterface) ENIInfo {
	return ENIInfo{
		eni: eni,
	}
}

func NewENIInfoViaInstanceENI(instanceENI *ec2.InstanceNetworkInterface) ENIInfo {
	return ENIInfo{
		instanceENI: instanceENI,
	}
}

func (e *ENIInfo) SecurityGroups() []string {
	var groups []*ec2.GroupIdentifier
	if e.eni != nil {
		groups = e.eni.Groups
	} else {
		groups = e.instanceENI.Groups
	}
	result := sets.NewString()
	for _, group := range groups {
		result.Insert(aws.StringValue(group.GroupId))
	}
	return result.List()
}

// TargetENIsResolver resolves the ENIs that supports targets for target groups.
type TargetENIsResolver interface {
	// Resolve returns ENIs that supports targets for target groups.
	Resolve(ctx context.Context, tgGroup tg.TargetGroupGroup) (map[string]ENIInfo, error)
}

func NewTargetENIsResolver(store store.Storer, cloud aws.CloudAPI) TargetENIsResolver {
	return &defaultTargetENIsResolver{
		store: store,
		cloud: cloud,
	}
}

type defaultTargetENIsResolver struct {
	store store.Storer
	cloud aws.CloudAPI
}

func (r *defaultTargetENIsResolver) Resolve(ctx context.Context, tgGroup tg.TargetGroupGroup) (map[string]ENIInfo, error) {
	targetInstances := sets.NewString()
	targetIPs := sets.NewString()
	for _, tg := range tgGroup.TGByBackend {
		if tg.TargetType == elbv2.TargetTypeEnumInstance {
			for _, endpoint := range tg.Targets {
				targetInstances.Insert(aws.StringValue(endpoint.Id))
			}
		} else {
			for _, endpoint := range tg.Targets {
				targetIPs.Insert(aws.StringValue(endpoint.Id))
			}
		}
	}

	targetENIs := make(map[string]ENIInfo)
	if targetInstances.Len() != 0 {
		targetENIsByInstance, err := r.findENIsSupportingInstanceTarget(ctx, targetInstances)
		if err != nil {
			return nil, err
		}
		for eniID, eniInfo := range targetENIsByInstance {
			targetENIs[eniID] = eniInfo
		}
	}
	if targetIPs.Len() != 0 {
		targetENIsByIP, err := r.findENIsSupportingIPTarget(ctx, targetIPs)
		if err != nil {
			return nil, err
		}
		for eniID, eniInfo := range targetENIsByIP {
			targetENIs[eniID] = eniInfo
		}
	}
	return targetENIs, nil
}

func (r *defaultTargetENIsResolver) findENIsSupportingInstanceTarget(ctx context.Context, instanceIDs sets.String) (map[string]ENIInfo, error) {
	instances, err := r.cloud.GetInstancesByIDs(instanceIDs.List())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get instance targets")
	}
	targetENIs := make(map[string]ENIInfo, len(instances))
	for _, instance := range instances {
		primaryENI, err := r.findInstancePrimaryENI(instance.NetworkInterfaces)
		if err != nil {
			return nil, err
		}
		targetENIs[aws.StringValue(primaryENI.NetworkInterfaceId)] = NewENIInfoViaInstanceENI(primaryENI)
	}
	return targetENIs, nil
}

func (r *defaultTargetENIsResolver) findENIsSupportingIPTarget(ctx context.Context, ips sets.String) (map[string]ENIInfo, error) {
	// we'll add another vpc filter, so minus 1
	ipChunks := utils.SplitStringSlice(ips.List(), EC2DescribeNetworkInterfacesFilterLimit-1)

	targetENIs := make(map[string]ENIInfo)
	for _, ipChunk := range ipChunks {
		enis, err := r.cloud.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{r.cloud.GetVpcID()}),
				},
				{
					Name:   aws.String("addresses.private-ip-address"),
					Values: aws.StringSlice(ipChunk),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		for _, eni := range enis {
			targetENIs[aws.StringValue(eni.NetworkInterfaceId)] = NewENIInfoViaENI(eni)
		}
	}
	return targetENIs, nil
}

// findInstancePrimaryENI returns the ID of primary ENI among list of eni on an EC2 instance
func (r *defaultTargetENIsResolver) findInstancePrimaryENI(enis []*ec2.InstanceNetworkInterface) (*ec2.InstanceNetworkInterface, error) {
	for _, eni := range enis {
		if aws.Int64Value(eni.Attachment.DeviceIndex) == 0 {
			return eni, nil
		}
	}
	return nil, errors.Errorf("[this should never happen] no primary ENI found")
}
