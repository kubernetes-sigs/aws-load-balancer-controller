package ec2

import (
	"context"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

// NewSecurityGroupSynthesizer constructs new securityGroupSynthesizer.
func NewSecurityGroupSynthesizer(ec2Client services.EC2, trackingProvider tracking.Provider, taggingManager TaggingManager,
	sgManager SecurityGroupManager, vpcID string, logger logr.Logger, stack core.Stack) *securityGroupSynthesizer {
	return &securityGroupSynthesizer{
		ec2Client:        ec2Client,
		trackingProvider: trackingProvider,
		taggingManager:   taggingManager,
		sgManager:        sgManager,
		vpcID:            vpcID,
		logger:           logger,
		stack:            stack,
		unmatchedSDKSGs:  nil,
	}
}

type securityGroupSynthesizer struct {
	ec2Client        services.EC2
	trackingProvider tracking.Provider
	taggingManager   TaggingManager
	sgManager        SecurityGroupManager
	vpcID            string
	logger           logr.Logger

	stack           core.Stack
	unmatchedSDKSGs []networking.SecurityGroupInfo
}

func (s *securityGroupSynthesizer) Synthesize(ctx context.Context) error {
	var resSGs []*ec2model.SecurityGroup
	s.stack.ListResources(&resSGs)
	sdkSGs, err := s.findSDKSecurityGroups(ctx)
	if err != nil {
		return err
	}
	matchedResAndSDKSGs, unmatchedResSGs, unmatchedSDKSGs, err := matchResAndSDKSecurityGroups(resSGs, sdkSGs, s.trackingProvider.ResourceIDTagKey())
	if err != nil {
		return err
	}

	// For SecurityGroup, we delete unmatched ones during post synthesize.
	s.unmatchedSDKSGs = unmatchedSDKSGs

	for _, resSG := range unmatchedResSGs {
		sgStatus, err := s.sgManager.Create(ctx, resSG)
		if err != nil {
			if isInvalidGroupDuplicateError(err) {
				sdkSG, findErr := s.findExistingSecurityGroupByName(ctx, resSG.Spec.GroupName)
				if findErr != nil {
					return errors.Wrapf(err, "Create failed with Duplicate and finding existing SG by name failed: %v", findErr)
				}
				if sdkSG != nil {
					s.logger.Info("adopting existing security group after Duplicate", "groupName", resSG.Spec.GroupName, "securityGroupID", sdkSG.SecurityGroupID)
					sgStatus, updateErr := s.sgManager.Update(ctx, resSG, *sdkSG)
					if updateErr != nil {
						return errors.Wrapf(updateErr, "failed to update adopted security group %s", sdkSG.SecurityGroupID)
					}
					resSG.SetStatus(sgStatus)
					continue
				}
			}
			return err
		}
		resSG.SetStatus(sgStatus)
	}
	for _, resAndSDKSG := range matchedResAndSDKSGs {
		sgStatus, err := s.sgManager.Update(ctx, resAndSDKSG.resSG, resAndSDKSG.sdkSG)
		if err != nil {
			return err
		}
		resAndSDKSG.resSG.SetStatus(sgStatus)
	}
	return nil
}

func (s *securityGroupSynthesizer) PostSynthesize(ctx context.Context) error {
	for _, sdkSG := range s.unmatchedSDKSGs {
		if err := s.sgManager.Delete(ctx, sdkSG); err != nil {
			return err
		}
	}
	return nil
}

// findSDKSecurityGroups will find all AWS SecurityGroups created for stack.
func (s *securityGroupSynthesizer) findSDKSecurityGroups(ctx context.Context) ([]networking.SecurityGroupInfo, error) {
	stackTags := s.trackingProvider.StackTags(s.stack)
	stackTagsLegacy := s.trackingProvider.StackTagsLegacy(s.stack)
	return s.taggingManager.ListSecurityGroups(ctx,
		tracking.TagsAsTagFilter(stackTags),
		tracking.TagsAsTagFilter(stackTagsLegacy))
}

type resAndSDKSecurityGroupPair struct {
	resSG *ec2model.SecurityGroup
	sdkSG networking.SecurityGroupInfo
}

func matchResAndSDKSecurityGroups(resSGs []*ec2model.SecurityGroup, sdkSGs []networking.SecurityGroupInfo,
	resourceIDTagKey string) ([]resAndSDKSecurityGroupPair, []*ec2model.SecurityGroup, []networking.SecurityGroupInfo, error) {
	var matchedResAndSDKSGs []resAndSDKSecurityGroupPair
	var unmatchedResSGs []*ec2model.SecurityGroup
	var unmatchedSDKSGs []networking.SecurityGroupInfo

	resSGsByID := mapResSecurityGroupByResourceID(resSGs)
	sdkSGsByID, err := mapSDKSecurityGroupByResourceID(sdkSGs, resourceIDTagKey)
	if err != nil {
		return nil, nil, nil, err
	}

	resSGIDs := sets.StringKeySet(resSGsByID)
	sdkSGIDs := sets.StringKeySet(sdkSGsByID)
	for _, resID := range resSGIDs.Intersection(sdkSGIDs).List() {
		resSG := resSGsByID[resID]
		sdkSGs := sdkSGsByID[resID]
		matchedResAndSDKSGs = append(matchedResAndSDKSGs, resAndSDKSecurityGroupPair{
			resSG: resSG,
			sdkSG: sdkSGs[0],
		})
		for _, sdkSG := range sdkSGs[1:] {
			unmatchedSDKSGs = append(unmatchedSDKSGs, sdkSG)
		}
	}
	for _, resID := range resSGIDs.Difference(sdkSGIDs).List() {
		unmatchedResSGs = append(unmatchedResSGs, resSGsByID[resID])
	}
	for _, resID := range sdkSGIDs.Difference(resSGIDs).List() {
		unmatchedSDKSGs = append(unmatchedSDKSGs, sdkSGsByID[resID]...)
	}

	return matchedResAndSDKSGs, unmatchedResSGs, unmatchedSDKSGs, nil
}

func mapResSecurityGroupByResourceID(resSGs []*ec2model.SecurityGroup) map[string]*ec2model.SecurityGroup {
	resSGsByID := make(map[string]*ec2model.SecurityGroup, len(resSGs))
	for _, resSG := range resSGs {
		resSGsByID[resSG.ID()] = resSG
	}
	return resSGsByID
}

func mapSDKSecurityGroupByResourceID(sdkSGs []networking.SecurityGroupInfo, resourceIDTagKey string) (map[string][]networking.SecurityGroupInfo, error) {
	sdkSGsByID := make(map[string][]networking.SecurityGroupInfo, len(sdkSGs))
	for _, sdkSG := range sdkSGs {
		resourceID, ok := sdkSG.Tags[resourceIDTagKey]
		if !ok {
			return nil, errors.Errorf("unexpected securityGroup with no resourceID: %v", sdkSG.SecurityGroupID)
		}
		sdkSGsByID[resourceID] = append(sdkSGsByID[resourceID], sdkSG)
	}
	return sdkSGsByID, nil
}

func isInvalidGroupDuplicateError(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "InvalidGroup.Duplicate"
	}
	return strings.Contains(err.Error(), "InvalidGroup.Duplicate")
}

// findExistingSecurityGroupByName describes SGs in the synthesizer's VPC by group name and returns the first match, or nil if not found.
func (s *securityGroupSynthesizer) findExistingSecurityGroupByName(ctx context.Context, groupName string) (*networking.SecurityGroupInfo, error) {
	req := &ec2sdk.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{Name: awssdk.String("vpc-id"), Values: []string{s.vpcID}},
			{Name: awssdk.String("group-name"), Values: []string{groupName}},
		},
	}
	sgs, err := s.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(sgs) == 0 {
		return nil, nil
	}
	info := networking.NewRawSecurityGroupInfo(sgs[0])
	return &info, nil
}
