package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
	"github.com/aws/aws-sdk-go/service/shield"
	"github.com/aws/aws-sdk-go/service/shield/shieldiface"
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/aws/aws-sdk-go/service/wafregional/wafregionaliface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/ticketmaster/aws-sdk-go-cache/cache"
)

type CloudAPI interface {
	ACMAPI
	EC2API
	ELBV2API
	IAMAPI
	ResourceGroupsTaggingAPIAPI
	ShieldAPI
	WAFRegionalAPI

	GetClusterName() string
	GetVpcID() string
}

type Cloud struct {
	vpcID       string
	region      string
	clusterName string

	acm         acmiface.ACMAPI
	ec2         ec2iface.EC2API
	elbv2       elbv2iface.ELBV2API
	iam         iamiface.IAMAPI
	shield      shieldiface.ShieldAPI
	rgt         resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
	wafregional wafregionaliface.WAFRegionalAPI
}

// Initialize the global AWS clients.
// But due to huge number of aws clients, it's best to have one container AWS client that embed these aws clients.
// TODO: remove clusterName dependency
// TODO: remove mc dependency like https://github.com/kubernetes/kubernetes/blob/master/pkg/cloudprovider/providers/aws/aws_metrics.go
func New(cfg CloudConfig, clusterName string, mc metric.Collector, ce bool, cc *cache.Config) (CloudAPI, error) {
	metadataSession := session.Must(session.NewSession(aws.NewConfig()))
	metadata := ec2metadata.New(metadataSession)
	if len(cfg.VpcID) == 0 {
		vpcID, err := GetVpcIDFromEC2Metadata(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to introspect vpcID from ec2Metadata due to %v, specify --aws-vpc-id instead if ec2Metadata is unavailable", err)
		}
		cfg.VpcID = vpcID
	}
	if len(cfg.Region) == 0 {
		region, err := metadata.Region()
		if err != nil {
			return nil, fmt.Errorf("failed to introspect region from ec2Metadata due to %v, specify --aws-region instead if ec2Metadata is unavailable", err)
		}
		cfg.Region = region
	}

	awsCfg := aws.NewConfig().WithRegion(cfg.Region).WithSTSRegionalEndpoint(endpoints.RegionalSTSEndpoint).WithMaxRetries(cfg.APIMaxRetries)
	awsSession := NewSession(awsCfg, cfg.APIDebug, mc, ce, cc)
	return &Cloud{
		cfg.VpcID,
		cfg.Region,
		clusterName,
		acm.New(awsSession),
		ec2.New(awsSession),
		elbv2.New(awsSession),
		iam.New(awsSession),
		shield.New(awsSession, &aws.Config{Region: aws.String("us-east-1")}),
		resourcegroupstaggingapi.New(awsSession),
		wafregional.New(awsSession),
	}, nil
}

func (c *Cloud) GetClusterName() string {
	return c.clusterName
}

func (c *Cloud) GetVpcID() string {
	return c.vpcID
}
