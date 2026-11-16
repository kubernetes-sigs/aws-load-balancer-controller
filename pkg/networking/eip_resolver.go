package networking

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
)

const ec2FilterNameDomain = "domain"

//go:generate mockgen -destination=eip_resolver_mocks.go -package=networking sigs.k8s.io/aws-load-balancer-controller/v3/pkg/networking EIPResolver

// EIPResolver resolves Elastic IP allocation IDs for NLB subnet mappings.
type EIPResolver interface {
	ResolveForSubnets(ctx context.Context, tagFilters map[string]string, subnets []ec2types.Subnet) ([]string, error)
}

// NewDefaultEIPResolver constructs a new defaultEIPResolver.
func NewDefaultEIPResolver(ec2Client services.EC2) *defaultEIPResolver {
	return &defaultEIPResolver{
		ec2Client: ec2Client,
	}
}

type defaultEIPResolver struct {
	ec2Client services.EC2
}

var _ EIPResolver = &defaultEIPResolver{}

func (r *defaultEIPResolver) ResolveForSubnets(ctx context.Context, tagFilters map[string]string, subnets []ec2types.Subnet) ([]string, error) {
	if len(tagFilters) == 0 {
		return nil, fmt.Errorf("EIP discovery tags must not be empty")
	}
	if len(subnets) == 0 {
		return nil, fmt.Errorf("subnets must not be empty for EIP discovery")
	}

	addresses, err := r.listAddressesByTagFilters(ctx, tagFilters)
	if err != nil {
		return nil, fmt.Errorf("failed to list EIPs by discovery tags: %w", err)
	}

	addressesByAZ := make(map[string][]ec2types.Address)
	for _, addr := range addresses {
		az := awssdk.ToString(addr.NetworkBorderGroup)
		if az == "" {
			return nil, fmt.Errorf("discovered EIP %s has empty network border group", awssdk.ToString(addr.AllocationId))
		}
		addressesByAZ[az] = append(addressesByAZ[az], addr)
	}

	allocationIDs := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		subnetAZ := awssdk.ToString(subnet.AvailabilityZone)
		addrsInAZ := addressesByAZ[subnetAZ]
		if len(addrsInAZ) == 0 {
			return nil, fmt.Errorf("no EIP found for subnet %s in availability zone %s matching discovery tags", awssdk.ToString(subnet.SubnetId), subnetAZ)
		}
		if len(addrsInAZ) > 1 {
			return nil, fmt.Errorf("multiple EIPs found for availability zone %s matching discovery tags", subnetAZ)
		}
		addr := addrsInAZ[0]
		if err := validateDiscoveredEIP(addr); err != nil {
			return nil, err
		}
		allocationIDs = append(allocationIDs, awssdk.ToString(addr.AllocationId))
	}

	return allocationIDs, nil
}

func validateDiscoveredEIP(addr ec2types.Address) error {
	if addr.AllocationId == nil || awssdk.ToString(addr.AllocationId) == "" {
		return fmt.Errorf("discovered EIP has empty allocation ID")
	}
	if addr.AssociationId != nil && awssdk.ToString(addr.AssociationId) != "" {
		owner := awssdk.ToString(addr.NetworkInterfaceOwnerId)
		if owner != "" && owner != "amazon-elb" {
			return fmt.Errorf("EIP %s is associated with another resource", awssdk.ToString(addr.AllocationId))
		}
	}
	return nil
}

func (r *defaultEIPResolver) listAddressesByTagFilters(ctx context.Context, tagFilters map[string]string) ([]ec2types.Address, error) {
	req := &ec2sdk.DescribeAddressesInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String(ec2FilterNameDomain),
				Values: []string{"vpc"},
			},
		},
	}
	for _, key := range sets.StringKeySet(tagFilters).List() {
		req.Filters = append(req.Filters, ec2types.Filter{
			Name:   awssdk.String("tag:" + key),
			Values: []string{tagFilters[key]},
		})
	}
	return r.ec2Client.DescribeAddressesAsList(ctx, req)
}
