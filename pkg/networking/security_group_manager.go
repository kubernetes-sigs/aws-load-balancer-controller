package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sync"
	"time"
)

const (
	// we cache securityGroup's information by 10 minutes.
	defaultSGInfoCacheTTL = 10 * time.Minute
)

type FetchSGInfoOptions struct {
	// whether to ignore cache and reload SecurityGroup Info from AWS directly.
	ReloadIgnoringCache bool
}

// Apply FetchSGInfoOption options
func (opts *FetchSGInfoOptions) ApplyOptions(options ...FetchSGInfoOption) {
	for _, option := range options {
		option(opts)
	}
}

type FetchSGInfoOption func(opts *FetchSGInfoOptions)

// WithReloadIgnoringCache is a option that sets the ReloadIgnoringCache to true.
func WithReloadIgnoringCache() FetchSGInfoOption {
	return func(opts *FetchSGInfoOptions) {
		opts.ReloadIgnoringCache = true
	}
}

// SecurityGroupManager is an abstraction around EC2's SecurityGroup API.
type SecurityGroupManager interface {
	// FetchSGInfosByID will fetch SecurityGroupInfo with SecurityGroup IDs.
	FetchSGInfosByID(ctx context.Context, sgIDs []string, opts ...FetchSGInfoOption) (map[string]SecurityGroupInfo, error)

	// FetchSGInfosByRequest will fetch SecurityGroupInfo with raw DescribeSecurityGroupsInput request.
	FetchSGInfosByRequest(ctx context.Context, req *ec2sdk.DescribeSecurityGroupsInput) (map[string]SecurityGroupInfo, error)

	// AuthorizeSGIngress will authorize Ingress permissions to SecurityGroup.
	AuthorizeSGIngress(ctx context.Context, sgID string, permissions []IPPermissionInfo) error

	// RevokeSGIngress will revoke Ingress permissions from SecurityGroup.
	RevokeSGIngress(ctx context.Context, sgID string, permissions []IPPermissionInfo) error
}

// NewDefaultSecurityGroupManager constructs new defaultSecurityGroupManager.
func NewDefaultSecurityGroupManager(ec2Client services.EC2, logger logr.Logger) *defaultSecurityGroupManager {
	return &defaultSecurityGroupManager{
		ec2Client: ec2Client,
		logger:    logger,

		sgInfoCache:      cache.NewExpiring(),
		sgInfoCacheMutex: sync.RWMutex{},
		sgInfoCacheTTL:   defaultSGInfoCacheTTL,
	}
}

var _ SecurityGroupManager = &defaultSecurityGroupManager{}

// default implementation for SecurityGroupManager
type defaultSecurityGroupManager struct {
	ec2Client services.EC2
	logger    logr.Logger

	sgInfoCache      *cache.Expiring
	sgInfoCacheMutex sync.RWMutex
	sgInfoCacheTTL   time.Duration
}

func (m *defaultSecurityGroupManager) FetchSGInfosByID(ctx context.Context, sgIDs []string, opts ...FetchSGInfoOption) (map[string]SecurityGroupInfo, error) {
	fetchOpts := FetchSGInfoOptions{
		ReloadIgnoringCache: false,
	}
	fetchOpts.ApplyOptions(opts...)

	sgInfoByID := make(map[string]SecurityGroupInfo, len(sgIDs))
	if !fetchOpts.ReloadIgnoringCache {
		sgInfoByIDFromCache := m.fetchSGInfosFromCache(sgIDs)
		for sgID, sgInfo := range sgInfoByIDFromCache {
			sgInfoByID[sgID] = sgInfo
		}
	}

	sgIDsSet := sets.NewString(sgIDs...)
	fetchedSGIDsSet := sets.StringKeySet(sgInfoByID)
	unFetchedSGIDs := sgIDsSet.Difference(fetchedSGIDsSet).List()
	if len(unFetchedSGIDs) > 0 {
		req := &ec2sdk.DescribeSecurityGroupsInput{
			GroupIds: awssdk.StringSlice(unFetchedSGIDs),
		}
		sgInfoByIDFromAWS, err := m.fetchSGInfosFromAWS(ctx, req)
		if err != nil {
			return nil, err
		}
		m.saveSGInfosToCache(sgInfoByIDFromAWS)
		for sgID, sgInfo := range sgInfoByIDFromAWS {
			sgInfoByID[sgID] = sgInfo
		}
	}

	fetchedSGIDsSet = sets.StringKeySet(sgInfoByID)
	if !sgIDsSet.Equal(fetchedSGIDsSet) {
		return nil, errors.Errorf("couldn't fetch SecurityGroupInfos: %v", sgIDsSet.Difference(fetchedSGIDsSet).List())
	}
	return sgInfoByID, nil
}

func (m *defaultSecurityGroupManager) FetchSGInfosByRequest(ctx context.Context, req *ec2sdk.DescribeSecurityGroupsInput) (map[string]SecurityGroupInfo, error) {
	sgInfosByID, err := m.fetchSGInfosFromAWS(ctx, req)
	if err != nil {
		return nil, err
	}
	m.saveSGInfosToCache(sgInfosByID)
	return sgInfosByID, nil
}

func (m *defaultSecurityGroupManager) AuthorizeSGIngress(ctx context.Context, sgID string, permissions []IPPermissionInfo) error {
	sdkIPPermissions := buildSDKIPPermissions(permissions)
	req := &ec2sdk.AuthorizeSecurityGroupIngressInput{
		GroupId:       awssdk.String(sgID),
		IpPermissions: sdkIPPermissions,
	}
	m.logger.Info("authorizing securityGroup ingress",
		"securityGroupID", sgID,
		"permission", sdkIPPermissions)
	if _, err := m.ec2Client.AuthorizeSecurityGroupIngressWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("authorized securityGroup ingress",
		"securityGroupID", sgID)

	// TODO: ideally we can remember the permissions we granted to save DescribeSecurityGroup API calls.
	m.clearSGInfosFromCache(sgID)
	return nil
}

func (m *defaultSecurityGroupManager) RevokeSGIngress(ctx context.Context, sgID string, permissions []IPPermissionInfo) error {
	sdkIPPermissions := buildSDKIPPermissions(permissions)
	req := &ec2sdk.RevokeSecurityGroupIngressInput{
		GroupId:       awssdk.String(sgID),
		IpPermissions: sdkIPPermissions,
	}
	m.logger.Info("revoking securityGroup ingress",
		"securityGroupID", sgID,
		"permission", sdkIPPermissions)
	if _, err := m.ec2Client.RevokeSecurityGroupIngressWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("revoked securityGroup ingress",
		"securityGroupID", sgID)

	// TODO: ideally we can remember the permissions we revoked to save DescribeSecurityGroup API calls.
	m.clearSGInfosFromCache(sgID)
	return nil
}

func (m *defaultSecurityGroupManager) fetchSGInfosFromCache(sgIDs []string) map[string]SecurityGroupInfo {
	m.sgInfoCacheMutex.RLock()
	defer m.sgInfoCacheMutex.RUnlock()

	sgInfoByID := make(map[string]SecurityGroupInfo, len(sgIDs))
	for _, sgID := range sgIDs {
		if rawCacheItem, exists := m.sgInfoCache.Get(sgID); exists {
			sgInfo := rawCacheItem.(SecurityGroupInfo)
			sgInfoByID[sgID] = sgInfo
		}
	}

	return sgInfoByID
}

func (m *defaultSecurityGroupManager) saveSGInfosToCache(sgInfoByID map[string]SecurityGroupInfo) {
	m.sgInfoCacheMutex.Lock()
	defer m.sgInfoCacheMutex.Unlock()

	for sgID, sgInfo := range sgInfoByID {
		m.sgInfoCache.Set(sgID, sgInfo, m.sgInfoCacheTTL)
	}
}

func (m *defaultSecurityGroupManager) clearSGInfosFromCache(sgID string) {
	m.sgInfoCache.Delete(sgID)
}

func (m *defaultSecurityGroupManager) fetchSGInfosFromAWS(ctx context.Context, req *ec2sdk.DescribeSecurityGroupsInput) (map[string]SecurityGroupInfo, error) {
	sgs, err := m.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	sgInfoByID := make(map[string]SecurityGroupInfo, len(sgs))
	for _, sg := range sgs {
		sgID := awssdk.StringValue(sg.GroupId)
		sgInfo := NewRawSecurityGroupInfo(sg)
		sgInfoByID[sgID] = sgInfo
	}
	return sgInfoByID, nil
}

// buildSDKIPPermissions converts slice of IPPermissionInfo into slice of pointers to IPPermission
// if targets is empty or nil, nil will be returned.
func buildSDKIPPermissions(permissions []IPPermissionInfo) []*ec2sdk.IpPermission {
	if len(permissions) == 0 {
		return nil
	}
	sdkPermissions := make([]*ec2sdk.IpPermission, 0, len(permissions))
	for i := range permissions {
		sdkPermissions = append(sdkPermissions, &permissions[i].Permission)
	}
	return sdkPermissions
}
