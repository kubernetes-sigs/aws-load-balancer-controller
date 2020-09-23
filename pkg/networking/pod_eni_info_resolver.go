package networking

import (
	"context"
	"encoding/json"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
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
	annotationPodENI = "vpc.amazonaws.com/pod-eni"

	defaultPodENIInfoCacheTTL = 10 * time.Minute
	// EC2:DescribeNetworkInterface supports up to 200 filters per call.
	describeNetworkInterfacesFiltersLimit = 200
)

// PodENIInfoResolver is responsible for resolve the AWS VPC ENI that supports pod network.
type PodENIInfoResolver interface {
	// Resolve resolves eniInfo for pods.
	Resolve(ctx context.Context, pods []*corev1.Pod) (map[types.NamespacedName]ENIInfo, error)
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

func (r *defaultPodENIInfoResolver) Resolve(ctx context.Context, pods []*corev1.Pod) (map[types.NamespacedName]ENIInfo, error) {
	eniInfoByPodKey := r.fetchENIInfosFromCache(pods)

	unresolvedPods := computePodsWithUnresolvedENIInfo(pods, eniInfoByPodKey)
	eniInfoByPodKeyViaLookup, err := r.resolveViaCascadedLookup(ctx, unresolvedPods)
	if err != nil {
		return nil, err
	}
	if len(eniInfoByPodKeyViaLookup) > 0 {
		r.saveENIInfosToCache(unresolvedPods, eniInfoByPodKeyViaLookup)
		for podKey, eniInfo := range eniInfoByPodKeyViaLookup {
			eniInfoByPodKey[podKey] = eniInfo
		}
		unresolvedPods = computePodsWithUnresolvedENIInfo(unresolvedPods, eniInfoByPodKeyViaLookup)
	}

	if len(unresolvedPods) > 0 {
		unresolvedPodKeys := make([]types.NamespacedName, 0, len(unresolvedPods))
		for _, pod := range unresolvedPods {
			unresolvedPodKeys = append(unresolvedPodKeys, k8s.NamespacedName(pod))
		}
		return nil, errors.Errorf("cannot resolve pod ENI for pods: %v", unresolvedPodKeys)
	}
	return eniInfoByPodKey, nil
}

type podENIInfoCacheKey struct {
	// Pod's key
	podKey types.NamespacedName
	// Pod's UID.
	// Note: we assume pod's eni haven't changed as long as pod UID is same.
	podUID string
}

func (r *defaultPodENIInfoResolver) fetchENIInfosFromCache(pods []*corev1.Pod) map[types.NamespacedName]ENIInfo {
	r.podENIInfoCacheMutex.RLock()
	defer r.podENIInfoCacheMutex.RUnlock()

	eniInfoByPodKey := make(map[types.NamespacedName]ENIInfo)
	for _, pod := range pods {
		cacheKey := computePodENIInfoCacheKey(pod)
		if rawCacheItem, exists := r.podENIInfoCache.Get(cacheKey); exists {
			eniInfo := rawCacheItem.(ENIInfo)
			podKey := k8s.NamespacedName(pod)
			eniInfoByPodKey[podKey] = eniInfo
		}
	}
	return eniInfoByPodKey
}

func (r *defaultPodENIInfoResolver) saveENIInfosToCache(pods []*corev1.Pod, eniInfoByPodKey map[types.NamespacedName]ENIInfo) {
	r.podENIInfoCacheMutex.Lock()
	defer r.podENIInfoCacheMutex.Unlock()

	for _, pod := range pods {
		podKey := k8s.NamespacedName(pod)
		if eniInfo, exists := eniInfoByPodKey[podKey]; exists {
			cacheKey := computePodENIInfoCacheKey(pod)
			r.podENIInfoCache.Set(cacheKey, eniInfo, r.podENIInfoCacheTTL)
		}
	}
}

func (r *defaultPodENIInfoResolver) resolveViaCascadedLookup(ctx context.Context, pods []*corev1.Pod) (map[types.NamespacedName]ENIInfo, error) {
	resolveFuncs := []func(ctx context.Context, pods []*corev1.Pod) (map[types.NamespacedName]ENIInfo, error){
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
		pods = computePodsWithUnresolvedENIInfo(pods, resolvedENIInfoByPodKey)
	}
	return eniInfoByPodKey, nil
}

// annotationSchemaPodENI is a json convertible structure that stores the Branch ENI details that can be
// used by the CNI plugin or the component consuming the resource
type annotationSchemaPodENI struct {
	// ENIID is the network interface id of the branch interface
	ENIID string `json:"eniId"`
	// PrivateIP is the primary IP of the branch Network interface
	PrivateIP string `json:"privateIp"`
	// SubnetCIDR is the CIDR block of the subnet
	SubnetCIDR string `json:"subnetCidr"`
}

// resolveViaPodENIAnnotation tries to resolve a pod ENI via the branch ENI annotation.
func (r *defaultPodENIInfoResolver) resolveViaPodENIAnnotation(ctx context.Context, pods []*corev1.Pod) (map[types.NamespacedName]ENIInfo, error) {
	podKeysByENIID := make(map[string][]types.NamespacedName)
	for _, pod := range pods {
		podENIAnnotation, ok := pod.Annotations[annotationPodENI]
		if !ok {
			continue
		}
		var schema annotationSchemaPodENI
		if err := json.Unmarshal([]byte(podENIAnnotation), &schema); err != nil {
			return nil, err
		}
		eniID := schema.ENIID
		podKey := k8s.NamespacedName(pod)
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
func (r *defaultPodENIInfoResolver) resolveViaVPCIPAddress(ctx context.Context, pods []*corev1.Pod) (map[types.NamespacedName]ENIInfo, error) {
	podKeysByIP := make(map[string][]types.NamespacedName, len(pods))
	for _, pod := range pods {
		podKeysByIP[pod.Status.PodIP] = append(podKeysByIP[pod.Status.PodIP], k8s.NamespacedName(pod))
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
func computePodENIInfoCacheKey(pod *corev1.Pod) podENIInfoCacheKey {
	return podENIInfoCacheKey{
		podKey: k8s.NamespacedName(pod),
		podUID: string(pod.UID),
	}
}

// computePodsWithUnresolvedENIInfo computes pods that don't have resolvedENIInfo.
func computePodsWithUnresolvedENIInfo(pods []*corev1.Pod, eniInfoByPodKey map[types.NamespacedName]ENIInfo) []*corev1.Pod {
	unresolvedPods := make([]*corev1.Pod, 0, len(pods)-len(eniInfoByPodKey))
	for _, pod := range pods {
		podKey := k8s.NamespacedName(pod)
		if _, ok := eniInfoByPodKey[podKey]; !ok {
			unresolvedPods = append(unresolvedPods, pod)
		}
	}
	return unresolvedPods
}
