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

// NewEndpointServiceSynthesizer constructs new endpointServiceSynthesizer.
func NewEndpointServiceSynthesizer(ec2Client services.EC2, trackingProvider tracking.Provider, taggingManager TaggingManager,
	esManager EndpointServiceManager, vpcID string, logger logr.Logger, stack core.Stack) *endpointServiceSynthesizer {
	return &endpointServiceSynthesizer{
		ec2Client:        ec2Client,
		trackingProvider: trackingProvider,
		taggingManager:   taggingManager,
		esManager:        esManager,
		vpcID:            vpcID,
		logger:           logger,
		stack:            stack,
	}
}

type endpointServiceSynthesizer struct {
	ec2Client        services.EC2
	trackingProvider tracking.Provider
	taggingManager   TaggingManager
	esManager        EndpointServiceManager
	vpcID            string
	logger           logr.Logger

	stack           core.Stack
	unmatchedSDKESs []networking.VPCEndpointServiceInfo
}

func (s *endpointServiceSynthesizer) Synthesize(ctx context.Context) error {
	// The load balancer synthesizer creates and deletes in its synthesize
	// loop.  We need to make sure that we delete any VPC endpoint services
	// before a load balancer deletion is attempted so we also need to delete
	// in our synthesize loop and we need to make sure that our synthesize
	// loop is called before the load balancers.
	var resESs []*ec2model.VPCEndpointService
	s.stack.ListResources(&resESs)
	sdkESs, err := s.findSDKEndpointServices(ctx)
	if err != nil {
		return err
	}

	_, _, unmatchedSDKESs, err := matchResAndSDKEndpointServices(resESs, sdkESs, s.trackingProvider.ResourceIDTagKey())
	if err != nil {
		return err
	}

	// We delete before we create as we can only have a single VPC end point per LB
	for _, sdkES := range unmatchedSDKESs {
		if err := s.esManager.Delete(ctx, sdkES); err != nil {
			return errors.Wrap(err, "failed to delete VPCEndpointService")
		}
	}

	return nil
}

func (s *endpointServiceSynthesizer) PostSynthesize(ctx context.Context) error {
	// We need the load balancer to be created before we attempt to create
	// our VPC endpoint services.  The load balancer synthesizer creates
	// load balancers in its synthesize loop so we can safely create in ours
	// in our post synthesize loop.
	// We can't create in our synthesize loop as we must synthesize before the
	// load balancer synthesizer.

	var resESs []*ec2model.VPCEndpointService
	s.stack.ListResources(&resESs)
	sdkESs, err := s.findSDKEndpointServices(ctx)
	if err != nil {
		return err
	}

	matchedResAndSDKESs, unmatchedResESs, _, err := matchResAndSDKEndpointServices(resESs, sdkESs, s.trackingProvider.ResourceIDTagKey())
	if err != nil {
		return err
	}

	for _, resES := range unmatchedResESs {
		esStatus, err := s.esManager.Create(ctx, resES)
		if err != nil {
			return errors.Wrap(err, "failed to create VPCEndpointService")
		}
		resES.SetStatus(esStatus)
	}

	for _, pair := range matchedResAndSDKESs {
		esStatus, err := s.esManager.Update(ctx, pair.res, pair.sdk)
		if err != nil {
			return errors.Wrap(err, "failed to update VPCEndpointService")
		}
		pair.res.SetStatus(esStatus)
	}

	var resESPs []*ec2model.VPCEndpointServicePermissions
	err = s.stack.ListResources(&resESPs)
	if err != nil {
		return err
	}
	s.logger.Info("Permission to reconcile", "permission", resESPs)
	for _, permission := range resESPs {
		err = s.esManager.ReconcilePermissions(ctx, permission)
		if err != nil {
			return errors.Wrap(err, "failed to reconcile VPCEndpointServicePermissions")
		}
	}

	return nil
}

func (s *endpointServiceSynthesizer) findSDKEndpointServices(ctx context.Context) ([]networking.VPCEndpointServiceInfo, error) {
	stackTags := s.trackingProvider.StackTags(s.stack)
	stackTagsLegacy := s.trackingProvider.StackTagsLegacy(s.stack)

	return s.taggingManager.ListVPCEndpointServices(ctx,
		tracking.TagsAsTagFilter(stackTags),
		tracking.TagsAsTagFilter(stackTagsLegacy),
	)
}

type resAndSDKEndpointServicePair struct {
	res *ec2model.VPCEndpointService
	sdk networking.VPCEndpointServiceInfo
}

func matchResAndSDKEndpointServices(resESs []*ec2model.VPCEndpointService, sdkESs []networking.VPCEndpointServiceInfo,
	resourceIDTagKey string) ([]resAndSDKEndpointServicePair, []*ec2model.VPCEndpointService, []networking.VPCEndpointServiceInfo, error) {

	var matchedResAndSDKESs []resAndSDKEndpointServicePair

	var unmatchedResESs []*ec2model.VPCEndpointService

	var unmatchedSDKESs []networking.VPCEndpointServiceInfo

	resESsByID := mapResEndpointServiceByResourceID(resESs)

	sdkESsByID, err := mapSDKEndpointServiceByResourceID(sdkESs, resourceIDTagKey)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to map VPCEndpointServices by ID")
	}

	resESIDs := sets.StringKeySet(resESsByID)
	sdkESIDs := sets.StringKeySet(sdkESsByID)

	for _, resID := range resESIDs.Intersection(sdkESIDs).List() {
		resES := resESsByID[resID]
		sdkESs := sdkESsByID[resID]

		matchedResAndSDKESs = append(matchedResAndSDKESs, resAndSDKEndpointServicePair{
			res: resES,
			sdk: sdkESs[0],
		})

		for _, sdkES := range sdkESs[1:] {
			unmatchedSDKESs = append(unmatchedSDKESs, sdkES)
		}
	}

	for _, resID := range resESIDs.Difference(sdkESIDs).List() {
		unmatchedResESs = append(unmatchedResESs, resESsByID[resID])
	}

	for _, resID := range sdkESIDs.Difference(resESIDs).List() {
		unmatchedSDKESs = append(unmatchedSDKESs, sdkESsByID[resID]...)
	}

	return matchedResAndSDKESs, unmatchedResESs, unmatchedSDKESs, nil
}

func mapResEndpointServiceByResourceID(resESs []*ec2model.VPCEndpointService) map[string]*ec2model.VPCEndpointService {
	resESsByID := make(map[string]*ec2model.VPCEndpointService, len(resESs))
	for _, resES := range resESs {
		resESsByID[resES.ID()] = resES
	}

	return resESsByID
}

func mapSDKEndpointServiceByResourceID(sdkESs []networking.VPCEndpointServiceInfo,
	resourceIDTagKey string) (map[string][]networking.VPCEndpointServiceInfo, error) {
	sdkESsByID := make(map[string][]networking.VPCEndpointServiceInfo, len(sdkESs))

	for _, sdkES := range sdkESs {
		resourceID, ok := sdkES.Tags[resourceIDTagKey]
		if !ok {
			return nil, errors.Errorf("unexpected VPCEndpointService with no resourceID: %v", sdkES.ServiceID)
		}

		sdkESsByID[resourceID] = append(sdkESsByID[resourceID], sdkES)
	}

	return sdkESsByID, nil
}
