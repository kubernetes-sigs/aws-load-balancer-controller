package ec2

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

// NewElasticIPAddressSynthesizer constructs new elasticIPAddressSynthesizer.
func NewElasticIPAddressSynthesizer(ec2Client services.EC2, trackingProvider tracking.Provider, taggingManager TaggingManager,
	eipManager ElasticIPAddressManager, vpcID string, logger logr.Logger, stack core.Stack) *elasticIPAddressSynthesizer {
	return &elasticIPAddressSynthesizer{
		ec2Client:        ec2Client,
		trackingProvider: trackingProvider,
		taggingManager:   taggingManager,
		eipManager:       eipManager,
		vpcID:            vpcID,
		logger:           logger,
		stack:            stack,
		unmatchedSDKEIPs: nil,
	}
}

type elasticIPAddressSynthesizer struct {
	ec2Client        services.EC2
	trackingProvider tracking.Provider
	taggingManager   TaggingManager
	eipManager       ElasticIPAddressManager
	vpcID            string
	logger           logr.Logger

	stack            core.Stack
	unmatchedSDKEIPs []networking.ElasticIPAddressInfo
}

func (s *elasticIPAddressSynthesizer) Synthesize(ctx context.Context) error {
	var resEIPs []*ec2model.ElasticIPAddress
	s.stack.ListResources(&resEIPs)
	sdkEIPs, err := s.findSDKElasticIPAddresses(ctx)
	if err != nil {
		return err
	}
	matchedResAndSDKEIPs, unmatchedResEIPs, unmatchedSDKEIPs, err := matchResAndSDKElasticIPAddresss(resEIPs, sdkEIPs, s.trackingProvider.ResourceIDTagKey())
	if err != nil {
		return err
	}

	// For Elastic IP Addresses, we delete unmatched ones during post synthesize.
	s.unmatchedSDKEIPs = unmatchedSDKEIPs

	for _, resEIP := range unmatchedResEIPs {
		sgStatus, err := s.eipManager.Create(ctx, resEIP)
		if err != nil {
			return err
		}
		resEIP.SetStatus(sgStatus)
	}
	for _, resAndSDKEIP := range matchedResAndSDKEIPs {
		sgStatus, err := s.eipManager.Update(ctx, resAndSDKEIP.resEIP, resAndSDKEIP.sdkEIP)
		if err != nil {
			return err
		}
		resAndSDKEIP.resEIP.SetStatus(sgStatus)
	}
	return nil
}

func (s *elasticIPAddressSynthesizer) PostSynthesize(ctx context.Context) error {
	for _, sdkEIP := range s.unmatchedSDKEIPs {
		if err := s.eipManager.Delete(ctx, sdkEIP); err != nil {
			return err
		}
	}
	return nil
}

// findSDKSecurityGroups will find all AWS Elastic IP Addresses created for stack.
func (s *elasticIPAddressSynthesizer) findSDKElasticIPAddresses(ctx context.Context) ([]networking.ElasticIPAddressInfo, error) {
	stackTags := s.trackingProvider.StackTags(s.stack)
	stackTagsLegacy := s.trackingProvider.StackTagsLegacy(s.stack)
	return s.taggingManager.ListElasticIPAddresses(ctx,
		tracking.TagsAsTagFilter(stackTags),
		tracking.TagsAsTagFilter(stackTagsLegacy))
}

type resAndSDKElasticIPAddressPair struct {
	resEIP *ec2model.ElasticIPAddress
	sdkEIP networking.ElasticIPAddressInfo
}

func matchResAndSDKElasticIPAddresss(resEIPs []*ec2model.ElasticIPAddress, sdkEIPs []networking.ElasticIPAddressInfo,
	resourceIDTagKey string) ([]resAndSDKElasticIPAddressPair, []*ec2model.ElasticIPAddress, []networking.ElasticIPAddressInfo, error) {
	var matchedResAndSDKEIPs []resAndSDKElasticIPAddressPair
	var unmatchedResEIPs []*ec2model.ElasticIPAddress
	var unmatchedSDKEIPs []networking.ElasticIPAddressInfo

	resEIPsByID := mapResElasticIPAddressByResourceID(resEIPs)
	sdkEIPsByID, err := mapSDKElasticIPAddressByResourceID(sdkEIPs, resourceIDTagKey)
	if err != nil {
		return nil, nil, nil, err
	}

	resEIPIDs := sets.StringKeySet(resEIPsByID)
	sdkEIPIDs := sets.StringKeySet(sdkEIPsByID)
	for _, resID := range resEIPIDs.Intersection(sdkEIPIDs).List() {
		resEIP := resEIPsByID[resID]
		sdkEIPs := sdkEIPsByID[resID]
		matchedResAndSDKEIPs = append(matchedResAndSDKEIPs, resAndSDKElasticIPAddressPair{
			resEIP: resEIP,
			sdkEIP: sdkEIPs[0],
		})
		for _, sdkEIP := range sdkEIPs[1:] {
			unmatchedSDKEIPs = append(unmatchedSDKEIPs, sdkEIP)
		}
	}
	for _, resID := range resEIPIDs.Difference(sdkEIPIDs).List() {
		unmatchedResEIPs = append(unmatchedResEIPs, resEIPsByID[resID])
	}
	for _, resID := range sdkEIPIDs.Difference(resEIPIDs).List() {
		unmatchedSDKEIPs = append(unmatchedSDKEIPs, sdkEIPsByID[resID]...)
	}

	return matchedResAndSDKEIPs, unmatchedResEIPs, unmatchedSDKEIPs, nil
}

func mapResElasticIPAddressByResourceID(resEIPs []*ec2model.ElasticIPAddress) map[string]*ec2model.ElasticIPAddress {
	resEIPsByID := make(map[string]*ec2model.ElasticIPAddress, len(resEIPs))
	for _, resEIP := range resEIPs {
		resEIPsByID[resEIP.ID()] = resEIP
	}
	return resEIPsByID
}

func mapSDKElasticIPAddressByResourceID(sdkEIPs []networking.ElasticIPAddressInfo, resourceIDTagKey string) (map[string][]networking.ElasticIPAddressInfo, error) {
	sdkEIPsByID := make(map[string][]networking.ElasticIPAddressInfo, len(sdkEIPs))
	for _, sdkEIP := range sdkEIPs {
		resourceID, ok := sdkEIP.Tags[resourceIDTagKey]
		if !ok {
			return nil, errors.Errorf("unexpected EIP with no resourceID: %v", sdkEIP.AllocationID)
		}
		sdkEIPsByID[resourceID] = append(sdkEIPsByID[resourceID], sdkEIP)
	}
	return sdkEIPsByID, nil
}
