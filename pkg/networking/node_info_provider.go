package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

// NodeInfoProvider is responsible for providing nodeInfo for nodes.
// TODO: provide a cached implementation for nodeInfoProvider, it can accepts cachePolicy per function.
// e.g. when resolve pod's ENI, the cachePolicy can be node contains pod's IP and node's cache is fresher than pod's creationTime.
type NodeInfoProvider interface {
	// FetchNodeInstances provides EC2 instance information per k8s node.
	FetchNodeInstances(ctx context.Context, nodes []*corev1.Node) (map[types.NamespacedName]*ec2sdk.Instance, error)
}

// NewDefaultNodeInfoProvider constructs new defaultNodeInfoProvider.
func NewDefaultNodeInfoProvider(ec2Client services.EC2, logger logr.Logger) *defaultNodeInfoProvider {
	return &defaultNodeInfoProvider{
		ec2Client: ec2Client,
		logger:    logger,
	}
}

var _ NodeInfoProvider = &defaultNodeInfoProvider{}

// defaultNodeInfoProvider is default implementation for NodeInfoProvider
type defaultNodeInfoProvider struct {
	// ec2 client
	ec2Client services.EC2

	// logger
	logger logr.Logger
}

func (p *defaultNodeInfoProvider) FetchNodeInstances(ctx context.Context, nodes []*corev1.Node) (map[types.NamespacedName]*ec2sdk.Instance, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	nodeKeysByInstanceID := make(map[string][]types.NamespacedName, len(nodes))
	for _, node := range nodes {
		instanceID, err := k8s.ExtractNodeInstanceID(node)
		if err != nil {
			return nil, err
		}
		nodeKey := k8s.NamespacedName(node)
		nodeKeysByInstanceID[instanceID] = append(nodeKeysByInstanceID[instanceID], nodeKey)
	}
	instanceIDs := sets.StringKeySet(nodeKeysByInstanceID).List()
	req := &ec2sdk.DescribeInstancesInput{
		InstanceIds: awssdk.StringSlice(instanceIDs),
	}
	instances, err := p.ec2Client.DescribeInstancesAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	nodeInstanceByNodeKey := make(map[types.NamespacedName]*ec2sdk.Instance, len(nodes))
	for _, instance := range instances {
		instanceID := awssdk.StringValue(instance.InstanceId)
		for _, nodeKey := range nodeKeysByInstanceID[instanceID] {
			nodeInstanceByNodeKey[nodeKey] = instance
		}
	}
	return nodeInstanceByNodeKey, nil
}
