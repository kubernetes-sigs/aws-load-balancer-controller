package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sync"
	"time"
)

const (
	defaultNodeENIInfoCacheTTL = 10 * time.Minute
)

// NodeENIInfoResolver is responsible for resolve the AWS VPC ENI that supports node network.
type NodeENIInfoResolver interface {
	Resolve(ctx context.Context, nodes []*corev1.Node) (map[types.NamespacedName]ENIInfo, error)
}

// NewDefaultNodeENIInfoResolver constructs new defaultNodeENIInfoResolver.
func NewDefaultNodeENIInfoResolver(ec2Client services.EC2, logger logr.Logger) *defaultNodeENIInfoResolver {
	return &defaultNodeENIInfoResolver{
		ec2Client:             ec2Client,
		logger:                logger,
		nodeENIInfoCache:      cache.NewExpiring(),
		nodeENIInfoCacheMutex: sync.RWMutex{},
		nodeENIInfoCacheTTL:   defaultNodeENIInfoCacheTTL,
	}
}

var _ NodeENIInfoResolver = &defaultNodeENIInfoResolver{}

// default implementation for NodeENIInfoResolver.
type defaultNodeENIInfoResolver struct {
	// ec2 client
	ec2Client services.EC2
	// logger
	logger logr.Logger

	nodeENIInfoCache      *cache.Expiring
	nodeENIInfoCacheMutex sync.RWMutex
	nodeENIInfoCacheTTL   time.Duration
}

func (r *defaultNodeENIInfoResolver) Resolve(ctx context.Context, nodes []*corev1.Node) (map[types.NamespacedName]ENIInfo, error) {
	eniInfoByNodeKey := r.fetchENIInfosFromCache(nodes)
	nodesWithoutENIInfo := computeNodesWithoutENIInfo(nodes, eniInfoByNodeKey)
	eniInfoByNodeKeyViaLookup, err := r.resolveViaInstanceID(ctx, nodesWithoutENIInfo)
	if err != nil {
		return nil, err
	}
	if len(eniInfoByNodeKeyViaLookup) > 0 {
		r.saveENIInfosToCache(nodesWithoutENIInfo, eniInfoByNodeKeyViaLookup)
		for nodeKey, eniInfo := range eniInfoByNodeKeyViaLookup {
			eniInfoByNodeKey[nodeKey] = eniInfo
		}
		nodesWithoutENIInfo = computeNodesWithoutENIInfo(nodesWithoutENIInfo, eniInfoByNodeKeyViaLookup)
	}

	if len(nodesWithoutENIInfo) > 0 {
		unresolvedNodeKeys := make([]types.NamespacedName, 0, len(nodesWithoutENIInfo))
		for _, node := range nodesWithoutENIInfo {
			unresolvedNodeKeys = append(unresolvedNodeKeys, k8s.NamespacedName(node))
		}
		return nil, errors.Errorf("cannot resolve node ENI for nodes: %v", unresolvedNodeKeys)
	}
	return eniInfoByNodeKey, nil
}

type nodeENIInfoCacheKey struct {
	// Node's key
	nodeKey types.NamespacedName
	// Node's UID.
	// Note: we assume node's eni haven't changed as long as node UID is same.
	nodeUID types.UID
}

func (r *defaultNodeENIInfoResolver) fetchENIInfosFromCache(nodes []*corev1.Node) map[types.NamespacedName]ENIInfo {
	r.nodeENIInfoCacheMutex.RLock()
	defer r.nodeENIInfoCacheMutex.RUnlock()

	eniInfoByNodeKey := make(map[types.NamespacedName]ENIInfo)
	for _, node := range nodes {
		cacheKey := computeNodeENIInfoCacheKey(node)
		if rawCacheItem, exists := r.nodeENIInfoCache.Get(cacheKey); exists {
			eniInfo := rawCacheItem.(ENIInfo)
			nodeKey := k8s.NamespacedName(node)
			eniInfoByNodeKey[nodeKey] = eniInfo
		}
	}
	return eniInfoByNodeKey
}

func (r *defaultNodeENIInfoResolver) saveENIInfosToCache(nodes []*corev1.Node, eniInfoByNodeKey map[types.NamespacedName]ENIInfo) {
	r.nodeENIInfoCacheMutex.Lock()
	defer r.nodeENIInfoCacheMutex.Unlock()

	for _, node := range nodes {
		nodeKey := k8s.NamespacedName(node)
		if eniInfo, exists := eniInfoByNodeKey[nodeKey]; exists {
			cacheKey := computeNodeENIInfoCacheKey(node)
			r.nodeENIInfoCache.Set(cacheKey, eniInfo, r.nodeENIInfoCacheTTL)
		}
	}
}

func (r *defaultNodeENIInfoResolver) resolveViaInstanceID(ctx context.Context, nodes []*corev1.Node) (map[types.NamespacedName]ENIInfo, error) {
	nodeKeysByInstanceID := make(map[string][]types.NamespacedName)
	for _, node := range nodes {
		instanceID, err := k8s.ExtractNodeInstanceID(node)
		if err != nil {
			return nil, err
		}
		nodeKey := k8s.NamespacedName(node)
		nodeKeysByInstanceID[instanceID] = append(nodeKeysByInstanceID[instanceID], nodeKey)
	}
	if len(nodeKeysByInstanceID) == 0 {
		return nil, nil
	}

	instanceIDs := sets.StringKeySet(nodeKeysByInstanceID).List()
	req := &ec2sdk.DescribeInstancesInput{
		InstanceIds: awssdk.StringSlice(instanceIDs),
	}
	instances, err := r.ec2Client.DescribeInstancesAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	eniInfoByNodeKey := make(map[types.NamespacedName]ENIInfo)
	for _, instance := range instances {
		instanceID := awssdk.StringValue(instance.InstanceId)
		primaryENI, err := findInstancePrimaryENI(instance.NetworkInterfaces)
		if err != nil {
			return nil, err
		}
		eniInfo := buildENIInfoViaInstanceENI(primaryENI)
		for _, nodeKey := range nodeKeysByInstanceID[instanceID] {
			eniInfoByNodeKey[nodeKey] = eniInfo
		}
	}
	return eniInfoByNodeKey, nil
}

// findInstancePrimaryENI returns the primary ENI among list of eni on an EC2 instance
func findInstancePrimaryENI(enis []*ec2sdk.InstanceNetworkInterface) (*ec2sdk.InstanceNetworkInterface, error) {
	for _, eni := range enis {
		if awssdk.Int64Value(eni.Attachment.DeviceIndex) == 0 {
			return eni, nil
		}
	}
	return nil, errors.Errorf("[this should never happen] no primary ENI found")
}

// computeNodeENIInfoCacheKey computes the cacheKey for node's ENIInfo cache.
func computeNodeENIInfoCacheKey(node *corev1.Node) nodeENIInfoCacheKey {
	return nodeENIInfoCacheKey{
		nodeKey: k8s.NamespacedName(node),
		nodeUID: node.UID,
	}
}

// computeNodesWithoutENIInfo computes nodes that don't have resolvedENIInfo.
func computeNodesWithoutENIInfo(nodes []*corev1.Node, eniInfoByNodeKey map[types.NamespacedName]ENIInfo) []*corev1.Node {
	unresolvedNodes := make([]*corev1.Node, 0, len(nodes)-len(eniInfoByNodeKey))
	for _, node := range nodes {
		nodeKey := k8s.NamespacedName(node)
		if _, ok := eniInfoByNodeKey[nodeKey]; !ok {
			unresolvedNodes = append(unresolvedNodes, node)
		}
	}
	return unresolvedNodes
}
