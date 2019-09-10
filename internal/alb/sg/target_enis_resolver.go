package sg

import (
	"context"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TargetENIsResolver resolves the ENIs that supports targets for target groups.
type TargetENIsResolver interface {
	// Resolve returns ENIs that supports targets for target groups.
	Resolve(ctx context.Context, tgGroup tg.TargetGroupGroup) (map[string]*ec2.InstanceNetworkInterface, error)
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

func (r *defaultTargetENIsResolver) Resolve(ctx context.Context, tgGroup tg.TargetGroupGroup) (map[string]*ec2.InstanceNetworkInterface, error) {
	instanceENIs, err := r.findClusterInstanceENIs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cluster ENIs")
	}
	targetENIIDs := sets.NewString()
	for _, tg := range tgGroup.TGByBackend {
		if tg.TargetType == elbv2.TargetTypeEnumInstance {
			eniIDs, err := r.findENIsSupportingTargetGroupOfTypeInstance(instanceENIs, tg)
			if err != nil {
				return nil, err
			}
			targetENIIDs = targetENIIDs.Union(eniIDs)
		} else {
			eniIDs, err := r.findENIsSupportingTargetGroupOfTypeIP(instanceENIs, tg)
			if err != nil {
				return nil, err
			}
			targetENIIDs = targetENIIDs.Union(eniIDs)
		}
	}

	targetENIs := make(map[string]*ec2.InstanceNetworkInterface, len(targetENIIDs))
	for _, enis := range instanceENIs {
		for _, eni := range enis {
			if targetENIIDs.Has(aws.StringValue(eni.NetworkInterfaceId)) {
				targetENIs[aws.StringValue(eni.NetworkInterfaceId)] = eni
			}
		}
	}
	return targetENIs, nil
}

// findClusterInstanceENIs retrieves all ENIs attached to instances indexed by instanceID
func (r *defaultTargetENIsResolver) findClusterInstanceENIs() (map[string][]*ec2.InstanceNetworkInterface, error) {
	instanceIDs, err := r.store.GetClusterInstanceIDs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get instance IDs within cluster")
	}
	instances, err := r.cloud.GetInstancesByIDs(instanceIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get instances within cluster")
	}
	result := make(map[string][]*ec2.InstanceNetworkInterface)
	for _, instance := range instances {
		result[aws.StringValue(instance.InstanceId)] = instance.NetworkInterfaces
	}
	return result, nil
}

// findENIsSupportingTargetGroupOfTypeInstance find the ID of ENIs that are used to supporting ingress traffic to targetGroup with targetType instance.
// For targetType instance, traffic is routed into primary ENI of instance(eth0, i.e. deviceIndex == 0), other network interfaces are not used.
func (r *defaultTargetENIsResolver) findENIsSupportingTargetGroupOfTypeInstance(instanceENIs map[string][]*ec2.InstanceNetworkInterface, group tg.TargetGroup) (sets.String, error) {
	targetInstanceIDs := sets.NewString()
	for _, endpoint := range group.Targets {
		targetInstanceIDs.Insert(aws.StringValue(endpoint.Id))
	}
	targetENIs := sets.NewString()
	for instanceID, enis := range instanceENIs {
		if !targetInstanceIDs.Has(instanceID) {
			continue
		}
		targetENI, err := r.findInstancePrimaryENI(enis)
		if err != nil {
			return nil, err
		}
		targetENIs.Insert(targetENI)
	}
	return targetENIs, nil
}

// findENIsSupportingTargetGroupOfTypeIP find the ID of ENIs that are used to supporting ingress traffic to targetGroup with targetType IP.
// For targetType IP, traffic is routed into the ENI for specific pod IPs.
// Warning: this function only works under CNI implementations that use ENI for pod IPs such as amazon k8s cni.
func (r *defaultTargetENIsResolver) findENIsSupportingTargetGroupOfTypeIP(instanceENIs map[string][]*ec2.InstanceNetworkInterface, group tg.TargetGroup) (sets.String, error) {
	targetPodIPs := sets.NewString()
	for _, endpoint := range group.Targets {
		targetPodIPs.Insert(aws.StringValue(endpoint.Id))
	}
	targetENIs := sets.NewString()
	for _, enis := range instanceENIs {
		targetENIs = targetENIs.Union(r.findInstanceAmazonCNIPodENIs(enis, targetPodIPs))
	}

	return targetENIs, nil
}

// findInstancePrimaryENI returns the ID of primary ENI among list of eni on an EC2 instance
func (r *defaultTargetENIsResolver) findInstancePrimaryENI(enis []*ec2.InstanceNetworkInterface) (string, error) {
	for _, eni := range enis {
		if aws.Int64Value(eni.Attachment.DeviceIndex) == 0 {
			return aws.StringValue(eni.NetworkInterfaceId), nil
		}
	}
	return "", errors.Errorf("[this should never happen] no primary ENI found")
}

// findInstanceAmazonCNIPodENIs returns the ID of ENIs that supports podsIPs under amazon-k8s-cni on a EC2 instance
func (r *defaultTargetENIsResolver) findInstanceAmazonCNIPodENIs(enis []*ec2.InstanceNetworkInterface, podIPs sets.String) sets.String {
	podsENIs := sets.NewString()
	for _, eni := range enis {
		for _, addr := range eni.PrivateIpAddresses {
			if podIPs.Has(aws.StringValue(addr.PrivateIpAddress)) {
				podsENIs.Insert(aws.StringValue(eni.NetworkInterfaceId))
				break
			}
		}
	}
	return podsENIs
}
