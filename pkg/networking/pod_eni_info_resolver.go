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
	"net"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
	"time"
)

const (
	defaultPodENIInfoCacheTTL = 10 * time.Minute
	// EC2:DescribeNetworkInterface supports up to 200 filters per call.
	describeNetworkInterfacesFiltersLimit = 200

	labelEKSComputeType = "eks.amazonaws.com/compute-type"
)

// PodENIInfoResolver is responsible for resolve the AWS VPC ENI that supports pod network.
type PodENIInfoResolver interface {
	// Resolve resolves eniInfo for pods.
	Resolve(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error)
}

// NewDefaultPodENIInfoResolver constructs new defaultPodENIInfoResolver.
func NewDefaultPodENIInfoResolver(k8sClient client.Client, ec2Client services.EC2, nodeInfoProvider NodeInfoProvider, vpcID string, logger logr.Logger) *defaultPodENIInfoResolver {
	return &defaultPodENIInfoResolver{
		k8sClient:                            k8sClient,
		ec2Client:                            ec2Client,
		nodeInfoProvider:                     nodeInfoProvider,
		vpcID:                                vpcID,
		logger:                               logger,
		podENIInfoCache:                      cache.NewExpiring(),
		podENIInfoCacheMutex:                 sync.RWMutex{},
		podENIInfoCacheTTL:                   defaultPodENIInfoCacheTTL,
		describeNetworkInterfacesIPChunkSize: describeNetworkInterfacesFiltersLimit - 1, // we used 1 filter for VPC.
	}
}

var _ PodENIInfoResolver = &defaultPodENIInfoResolver{}

// default implementation for PodENIResolver
type defaultPodENIInfoResolver struct {
	// k8s client
	k8sClient client.Client
	// ec2 client
	ec2Client services.EC2
	// nodeInfoProvider
	nodeInfoProvider NodeInfoProvider
	// vpcID
	vpcID string
	// logger
	logger logr.Logger

	// cache of ENIInfo by podUID(podKey + UID).
	// NOTE: since this cache implementation will automatically GC expired entries, we don't need to GC entries.
	podENIInfoCache *cache.Expiring
	// podENICacheMutex protects podENICache
	podENIInfoCacheMutex sync.RWMutex
	// TTL for each cache entries.
	// Note: we assume pod's ENI information(e.g. securityGroups) haven't changed per podENICacheTTL.
	podENIInfoCacheTTL time.Duration

	// chunkSize when describe network interface with IPAddress filter.
	describeNetworkInterfacesIPChunkSize int
}

func (r *defaultPodENIInfoResolver) Resolve(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error) {
	eniInfoByPodKey := r.fetchENIInfosFromCache(pods)
	podsWithoutENIInfo := computePodsWithoutENIInfo(pods, eniInfoByPodKey)
	if len(podsWithoutENIInfo) > 0 {
		eniInfoByPodKeyViaLookup, err := r.resolveViaCascadedLookup(ctx, podsWithoutENIInfo)
		if err != nil {
			return nil, err
		}
		r.saveENIInfosToCache(podsWithoutENIInfo, eniInfoByPodKeyViaLookup)
		for podKey, eniInfo := range eniInfoByPodKeyViaLookup {
			eniInfoByPodKey[podKey] = eniInfo
		}
		podsWithoutENIInfo = computePodsWithoutENIInfo(podsWithoutENIInfo, eniInfoByPodKeyViaLookup)
	}

	if len(podsWithoutENIInfo) > 0 {
		podKeysWithoutENIInfo := make([]types.NamespacedName, 0, len(podsWithoutENIInfo))
		for _, pod := range podsWithoutENIInfo {
			podKeysWithoutENIInfo = append(podKeysWithoutENIInfo, pod.Key)
		}
		return nil, errors.Errorf("cannot resolve pod ENI for pods: %v", podKeysWithoutENIInfo)
	}
	return eniInfoByPodKey, nil
}

type podENIInfoCacheKey struct {
	// Pod's key
	podKey types.NamespacedName
	// Pod's UID.
	// Note: we assume pod's eni haven't changed as long as pod UID is same.
	podUID types.UID
}

func (r *defaultPodENIInfoResolver) fetchENIInfosFromCache(pods []k8s.PodInfo) map[types.NamespacedName]ENIInfo {
	r.podENIInfoCacheMutex.RLock()
	defer r.podENIInfoCacheMutex.RUnlock()

	eniInfoByPodKey := make(map[types.NamespacedName]ENIInfo)
	for _, pod := range pods {
		cacheKey := computePodENIInfoCacheKey(pod)
		if rawCacheItem, exists := r.podENIInfoCache.Get(cacheKey); exists {
			eniInfo := rawCacheItem.(ENIInfo)
			podKey := pod.Key
			eniInfoByPodKey[podKey] = eniInfo
		}
	}
	return eniInfoByPodKey
}

func (r *defaultPodENIInfoResolver) saveENIInfosToCache(pods []k8s.PodInfo, eniInfoByPodKey map[types.NamespacedName]ENIInfo) {
	r.podENIInfoCacheMutex.Lock()
	defer r.podENIInfoCacheMutex.Unlock()

	for _, pod := range pods {
		podKey := pod.Key
		if eniInfo, exists := eniInfoByPodKey[podKey]; exists {
			cacheKey := computePodENIInfoCacheKey(pod)
			r.podENIInfoCache.Set(cacheKey, eniInfo, r.podENIInfoCacheTTL)
		}
	}
}

func (r *defaultPodENIInfoResolver) resolveViaCascadedLookup(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error) {
	resolveFuncs := []func(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error){
		r.resolveViaPodENIAnnotation,
		r.resolveViaNodeENIs,
		r.resolveViaVPCENIs,
		// TODO, add support for kubenet CNI plugin(kops) by resolve via routeTable.
	}

	eniInfoByPodKey := make(map[types.NamespacedName]ENIInfo)
	for _, resolveFunc := range resolveFuncs {
		if len(pods) == 0 {
			break
		}

		resolvedENIInfoByPodKey, err := resolveFunc(ctx, pods)
		if err != nil {
			return nil, err
		}
		for podKey, eniInfo := range resolvedENIInfoByPodKey {
			eniInfoByPodKey[podKey] = eniInfo
		}
		pods = computePodsWithoutENIInfo(pods, resolvedENIInfoByPodKey)
	}
	return eniInfoByPodKey, nil
}

// resolveViaPodENIAnnotation tries to resolve pod ENI by lookup pod's ENIInfo annotation.
// with aws-vpc-cni CNI plugin's SecurityGroups for pods feature, podIP is supported by branchENI, whose information is exposed as pod annotation.
func (r *defaultPodENIInfoResolver) resolveViaPodENIAnnotation(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error) {
	podKeysByENIID := make(map[string][]types.NamespacedName)
	for _, pod := range pods {
		var eniID string
		for _, podENIInfo := range pod.ENIInfos {
			if podENIInfo.PrivateIP == pod.PodIP {
				eniID = podENIInfo.ENIID
			}
		}
		if len(eniID) == 0 {
			continue
		}

		podKey := pod.Key
		podKeysByENIID[eniID] = append(podKeysByENIID[eniID], podKey)
	}
	if len(podKeysByENIID) == 0 {
		return nil, nil
	}

	eniIDs := sets.StringKeySet(podKeysByENIID).List()
	req := &ec2sdk.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: awssdk.StringSlice(eniIDs),
	}
	enis, err := r.ec2Client.DescribeNetworkInterfacesAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	eniInfoByPodKey := make(map[types.NamespacedName]ENIInfo)
	for _, eni := range enis {
		eniID := awssdk.StringValue(eni.NetworkInterfaceId)
		eniInfo := buildENIInfoViaENI(eni)
		for _, podKey := range podKeysByENIID[eniID] {
			eniInfoByPodKey[podKey] = eniInfo
		}
	}
	return eniInfoByPodKey, nil
}

// resolveViaNodeENIs tries to resolve Pod ENI by matching podIP against ENIs on EC2 node's ENIs.
// with aws-vpc-cni CNI plugin, podIP can be supported by either IPv4Addresses or IPv4Prefixes on ENI.
func (r *defaultPodENIInfoResolver) resolveViaNodeENIs(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error) {
	nodeKeysSet := make(map[types.NamespacedName]sets.Empty)
	for _, pod := range pods {
		nodeKey := types.NamespacedName{Name: pod.NodeName}
		nodeKeysSet[nodeKey] = sets.Empty{}
	}
	nodes := make([]*corev1.Node, 0, len(nodeKeysSet))
	for nodeKey := range nodeKeysSet {
		node := &corev1.Node{}
		if err := r.k8sClient.Get(ctx, nodeKey, node); err != nil {
			return nil, err
		}
		// Fargate based nodes are not EC2 instances
		if node.Labels[labelEKSComputeType] == "fargate" {
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	nodeInstanceByNodeKey, err := r.nodeInfoProvider.FetchNodeInstances(ctx, nodes)
	if err != nil {
		return nil, err
	}
	eniInfoByPodKey := make(map[types.NamespacedName]ENIInfo, len(pods))
	for _, pod := range pods {
		nodeKey := types.NamespacedName{Name: pod.NodeName}
		if nodeInstance, exists := nodeInstanceByNodeKey[nodeKey]; exists {
			for _, instanceENI := range nodeInstance.NetworkInterfaces {
				if r.isPodSupportedByNodeENI(pod, instanceENI) {
					eniInfoByPodKey[pod.Key] = buildENIInfoViaInstanceENI(instanceENI)
					break
				}
			}
		}
	}
	return eniInfoByPodKey, nil
}

// resolveViaVPCENIs tries to resolve pod ENI by matching podIP against ENIs in vpc.
// with EKS fargate pods, podIP is supported by an ENI in vpc.
func (r *defaultPodENIInfoResolver) resolveViaVPCENIs(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error) {
	podKeysByIP := make(map[string][]types.NamespacedName, len(pods))
	for _, pod := range pods {
		podKeysByIP[pod.PodIP] = append(podKeysByIP[pod.PodIP], pod.Key)
	}
	if len(podKeysByIP) == 0 {
		return nil, nil
	}
	var ipv4PodIPs []string
	var ipv6PodIPs []string
	for _, podIP := range sets.StringKeySet(podKeysByIP).List() {
		if !strings.Contains(podIP, ":") {
			ipv4PodIPs = append(ipv4PodIPs, podIP)
		} else {
			ipv6PodIPs = append(ipv6PodIPs, podIP)
		}
	}

	eniInfoByPodKey := make(map[types.NamespacedName]ENIInfo)
	if len(ipv4PodIPs) > 0 {
		eniByID, err := r.getENIMappingViaDescribe(ctx, ipv4PodIPs, "addresses.private-ip-address")
		if err != nil {
			return nil, err
		}
		for _, eni := range eniByID {
			eniInfo := buildENIInfoViaENI(eni)
			for _, addr := range eni.PrivateIpAddresses {
				eniIP := awssdk.StringValue(addr.PrivateIpAddress)
				for _, podKey := range podKeysByIP[eniIP] {
					eniInfoByPodKey[podKey] = eniInfo
				}
			}
		}
	}

	if len(ipv6PodIPs) > 0 {
		eniByID, err := r.getENIMappingViaDescribe(ctx, ipv6PodIPs, "ipv6-addresses.ipv6-address")
		if err != nil {
			return nil, err
		}
		for _, eni := range eniByID {
			eniInfo := buildENIInfoViaENI(eni)
			for _, addr := range eni.Ipv6Addresses {
				eniIPv6 := awssdk.StringValue(addr.Ipv6Address)
				for _, podKey := range podKeysByIP[eniIPv6] {
					eniInfoByPodKey[podKey] = eniInfo
				}
			}
		}
	}
	return eniInfoByPodKey, nil
}

func (r *defaultPodENIInfoResolver) getENIMappingViaDescribe(ctx context.Context, podIPs []string, ipAddressFilterKey string) (map[string]*ec2sdk.NetworkInterface, error) {
	podIPChunks := algorithm.ChunkStrings(podIPs, r.describeNetworkInterfacesIPChunkSize)
	eniByID := make(map[string]*ec2sdk.NetworkInterface)
	for _, podIPChunk := range podIPChunks {
		req := &ec2sdk.DescribeNetworkInterfacesInput{
			Filters: []*ec2sdk.Filter{
				{
					Name:   awssdk.String("vpc-id"),
					Values: awssdk.StringSlice([]string{r.vpcID}),
				},
				{
					Name:   awssdk.String(ipAddressFilterKey),
					Values: awssdk.StringSlice(podIPChunk),
				},
			},
		}
		enis, err := r.ec2Client.DescribeNetworkInterfacesAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, eni := range enis {
			eniID := awssdk.StringValue(eni.NetworkInterfaceId)
			eniByID[eniID] = eni
		}
	}
	return eniByID, nil
}

// isPodSupportedByNodeENI checks whether pod is supported by specific nodeENI.
func (r *defaultPodENIInfoResolver) isPodSupportedByNodeENI(pod k8s.PodInfo, nodeENI *ec2sdk.InstanceNetworkInterface) bool {
	for _, ipv4Address := range nodeENI.PrivateIpAddresses {
		if pod.PodIP == awssdk.StringValue(ipv4Address.PrivateIpAddress) {
			return true
		}
	}

	if len(nodeENI.Ipv4Prefixes) > 0 || len(nodeENI.Ipv6Prefixes) > 0 {
		if podIP := net.ParseIP(pod.PodIP); podIP != nil {
			for _, ipv4Prefix := range nodeENI.Ipv4Prefixes {
				if _, ipv4CIDR, err := net.ParseCIDR(awssdk.StringValue(ipv4Prefix.Ipv4Prefix)); err == nil && ipv4CIDR.Contains(podIP) {
					return true
				}
			}
			for _, ipv6Prefix := range nodeENI.Ipv6Prefixes {
				if _, ipv6CIDR, err := net.ParseCIDR(awssdk.StringValue(ipv6Prefix.Ipv6Prefix)); err == nil && ipv6CIDR.Contains(podIP) {
					return true
				}
			}
		}
	}

	return false
}

// computePodENIInfoCacheKey computes the cacheKey for pod's ENIInfo cache.
func computePodENIInfoCacheKey(podInfo k8s.PodInfo) podENIInfoCacheKey {
	return podENIInfoCacheKey{
		podKey: podInfo.Key,
		podUID: podInfo.UID,
	}
}

// computePodsWithoutENIInfo computes pods that don't have a ENIInfo.
func computePodsWithoutENIInfo(pods []k8s.PodInfo, eniInfoByPodKey map[types.NamespacedName]ENIInfo) []k8s.PodInfo {
	podsWithoutENIInfo := make([]k8s.PodInfo, 0, len(pods)-len(eniInfoByPodKey))
	for _, pod := range pods {
		podKey := pod.Key
		if _, ok := eniInfoByPodKey[podKey]; !ok {
			podsWithoutENIInfo = append(podsWithoutENIInfo, pod)
		}
	}
	return podsWithoutENIInfo
}
