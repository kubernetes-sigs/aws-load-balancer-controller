package services

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"io"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type EC2Metadata interface {
	Region() (string, error)
	VpcID() (string, error)
}

// NewEC2Metadata constructs new EC2Metadata implementation.
func NewEC2Metadata(cfg aws.Config, endpointsResolver *endpoints.Resolver) EC2Metadata {
	customEndpoint := endpointsResolver.EndpointFor(imds.ServiceID)
	return &ec2metadataClient{
		ec2metadataClient: imds.NewFromConfig(cfg, func(o *imds.Options) {
			if customEndpoint != nil {
				o.Endpoint = aws.ToString(customEndpoint)
			}
		}),
	}
}

type ec2metadataClient struct {
	ec2metadataClient *imds.Client
}

func (c *ec2metadataClient) Region() (string, error) {
	output, err := c.ec2metadataClient.GetRegion(context.TODO(), &imds.GetRegionInput{})
	if err != nil {
		return "", err
	}
	return output.Region, nil
}

func (c *ec2metadataClient) VpcID() (string, error) {
	mac, err := c.getMetadata("mac")
	if err != nil {
		return "", fmt.Errorf("error in fetching vpc id through ec2 metadata: %w", err)
	}
	vpcId, err := c.getMetadata(fmt.Sprintf("network/interfaces/macs/%s/vpc-id", mac))
	if err != nil {
		return "", fmt.Errorf("error in fetching vpc id through ec2 metadata: %w", err)
	}
	return vpcId, nil
}

func (c *ec2metadataClient) getMetadata(path string) (string, error) {
	data, err := c.ec2metadataClient.GetMetadata(context.TODO(), &imds.GetMetadataInput{Path: path})
	if err != nil {
		return "", fmt.Errorf("get %s metadata: %w", path, err)
	}
	out, err := io.ReadAll(data.Content)
	if err != nil {
		return "", fmt.Errorf("read %s metadata: %w", path, err)
	}
	return string(out), nil
}
