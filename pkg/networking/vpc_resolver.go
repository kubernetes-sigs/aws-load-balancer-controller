package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// VPCResolver is responsible to resolve VPC information
type VPCResolver interface {
	// ResolveCIDRs resolves the VPC IPv4 CIDRs
	ResolveCIDRs(ctx context.Context) ([]string, error)

	// ResolveIPv6CIDRs resolves the VPC IPv6 CIDRs
	ResolveIPv6CIDRs(ctx context.Context) ([]string, error)
}

// NewDefaultVPCResolver constructs a new defaultVPCResolver
func NewDefaultVPCResolver(ec2Client services.EC2, vpcID string, logger logr.Logger) *defaultVPCResolver {
	return &defaultVPCResolver{
		ec2Client: ec2Client,
		vpcID:     vpcID,
		logger:    logger,
	}
}

var _ VPCResolver = &defaultVPCResolver{}

// default implementation for VPCResolver
type defaultVPCResolver struct {
	ec2Client services.EC2
	vpcID     string
	logger    logr.Logger
}

func (r *defaultVPCResolver) ResolveCIDRs(ctx context.Context) ([]string, error) {
	vpc, err := r.getVPCFromID(ctx, r.vpcID)
	if err != nil {
		return nil, err
	}
	var vpcCIDRs []string
	for _, cidr := range vpc.CidrBlockAssociationSet {
		vpcCIDRs = append(vpcCIDRs, awssdk.StringValue(cidr.CidrBlock))
	}

	return vpcCIDRs, nil
}

func (r *defaultVPCResolver) ResolveIPv6CIDRs(ctx context.Context) ([]string, error) {
	vpc, err := r.getVPCFromID(ctx, r.vpcID)
	if err != nil {
		return nil, err
	}
	var vpcIPv6CIDRs []string
	for _, cidr := range vpc.Ipv6CidrBlockAssociationSet {
		vpcIPv6CIDRs = append(vpcIPv6CIDRs, awssdk.StringValue(cidr.Ipv6CidrBlock))
	}
	return vpcIPv6CIDRs, nil
}

func (r *defaultVPCResolver) getVPCFromID(ctx context.Context, vpcID string) (*ec2.Vpc, error) {
	vpcs, err := r.ec2Client.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
		VpcIds: []*string{awssdk.String(vpcID)},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to describe VPC")
	}
	if len(vpcs.Vpcs) == 0 {
		return nil, errors.Errorf("unable to find matching VPC %q", r.vpcID)
	}
	return vpcs.Vpcs[0], nil
}
