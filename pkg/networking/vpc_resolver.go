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
	// ResolveCIDRs resolves the VPC CIDRs
	ResolveCIDRs(ctx context.Context) ([]string, error)
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
	vpcs, err := r.ec2Client.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
		VpcIds: []*string{awssdk.String(r.vpcID)},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to describe VPC")
	}
	if len(vpcs.Vpcs) == 0 {
		return nil, errors.Errorf("unable to find matching VPC %q", r.vpcID)
	}
	cidrBlockAssociationSet := vpcs.Vpcs[0].CidrBlockAssociationSet
	var vpcCIDRs []string

	for _, cidr := range cidrBlockAssociationSet {
		vpcCIDRs = append(vpcCIDRs, awssdk.StringValue(cidr.CidrBlock))
	}

	return vpcCIDRs, nil
}
