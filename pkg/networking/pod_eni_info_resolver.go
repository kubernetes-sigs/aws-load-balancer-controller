package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sync"
	"time"
)

const (
	defaultPodENIInfoCacheTTL = 10 * time.Minute
	// EC2:DescribeNetworkInterface supports up to 200 filters per call.
	describeNetworkInterfacesFiltersLimit = 200
)

// PodENIInfoResolver is responsible for resolve the AWS VPC ENI that supports pod network.
type PodENIInfoResolver interface {
	// Resolve resolves eniInfo for pods.
	Resolve(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error)
}

// NewDefaultPodENIResolver constructs new defaultPodENIResolver.
func NewDefaultPodENIInfoResolver(ec2Client services.EC2, vpcID string, logger logr.Logger) *defaultPodENIInfoResolver {
	return &defaultPodENIInfoResolver{
		ec2Client:                            ec2Client,
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
	// ec2 client
	ec2Client services.EC2
	// our vpcID.
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

	describeNetworkInterfacesIPChunkSize int
}

func (r *defaultPodENIInfoResolver) Resolve(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error) {
	eniInfoByPodKey := r.fetchENIInfosFromCache(pods)
	podsWithoutENIInfo := computePodsWithoutENIInfo(pods, eniInfoByPodKey)
	eniInfoByPodKeyViaLookup, err := r.resolveViaCascadedLookup(ctx, podsWithoutENIInfo)
	if err != nil {
		return nil, err
	}
	if len(eniInfoByPodKeyViaLookup) > 0 {
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
		r.resolveViaVPCIPAddress,
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

// resolveViaPodENIAnnotation tries to resolve a pod ENI via the branch ENI annotation.
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

// resolveViaVPCIPAddress tries to resolve pod ENI via the IPAddress within VPC.
func (r *defaultPodENIInfoResolver) resolveViaVPCIPAddress(ctx context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]ENIInfo, error) {
	podKeysByIP := make(map[string][]types.NamespacedName, len(pods))
	for _, pod := range pods {
		podKeysByIP[pod.PodIP] = append(podKeysByIP[pod.PodIP], pod.Key)
	}
	if len(podKeysByIP) == 0 {
		return nil, nil
	}

	podIPs := sets.StringKeySet(podKeysByIP).List()
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
					Name:   awssdk.String("addresses.private-ip-address"),
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

	eniInfoByPodKey := make(map[types.NamespacedName]ENIInfo)
	for _, eni := range eniByID {
		eniInfo := buildENIInfoViaENI(eni)
		for _, addr := range eni.PrivateIpAddresses {
			eniIP := awssdk.StringValue(addr.PrivateIpAddress)
			for _, podKey := range podKeysByIP[eniIP] {
				eniInfoByPodKey[podKey] = eniInfo
			}
		}
	}
	return eniInfoByPodKey, nil
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
