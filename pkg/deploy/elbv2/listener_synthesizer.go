package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func NewListenerSynthesizer(elbv2Client services.ELBV2, taggingManager TaggingManager,
	lsManager ListenerManager, logger logr.Logger, stack core.Stack) *listenerSynthesizer {
	return &listenerSynthesizer{
		elbv2Client:    elbv2Client,
		lsManager:      lsManager,
		logger:         logger,
		taggingManager: taggingManager,
		stack:          stack,
	}
}

type listenerSynthesizer struct {
	elbv2Client    services.ELBV2
	lsManager      ListenerManager
	logger         logr.Logger
	taggingManager TaggingManager

	stack core.Stack
}

func (s *listenerSynthesizer) Synthesize(ctx context.Context) error {
	var resLSs []*elbv2model.Listener
	s.stack.ListResources(&resLSs)
	resLSsByLBARN, err := mapResListenerByLoadBalancerARN(resLSs)
	if err != nil {
		return err
	}

	for lbARN, resLSs := range resLSsByLBARN {
		if err := s.synthesizeListenersOnLB(ctx, lbARN, resLSs); err != nil {
			return err
		}
	}
	return nil
}

func (s *listenerSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here.
	return nil
}

func (s *listenerSynthesizer) synthesizeListenersOnLB(ctx context.Context, lbARN string, resLSs []*elbv2model.Listener) error {
	sdkLSs, err := s.findSDKListenersOnLB(ctx, lbARN)
	if err != nil {
		return err
	}
	matchedResAndSDKLSs, unmatchedResLSs, unmatchedSDKLSs := matchResAndSDKListeners(resLSs, sdkLSs)
	for _, sdkLS := range unmatchedSDKLSs {
		if err := s.lsManager.Delete(ctx, sdkLS); err != nil {
			return err
		}
	}
	for _, resLS := range unmatchedResLSs {
		lsStatus, err := s.lsManager.Create(ctx, resLS)
		if err != nil {
			return err
		}
		resLS.SetStatus(lsStatus)
	}
	for _, resAndSDKLS := range matchedResAndSDKLSs {
		lsStatus, err := s.lsManager.Update(ctx, resAndSDKLS.resLS, resAndSDKLS.sdkLS)
		if err != nil {
			return err
		}
		resAndSDKLS.resLS.SetStatus(lsStatus)
	}
	return nil
}

// findSDKListenersOnLB returns the listeners configured on LoadBalancer.
func (s *listenerSynthesizer) findSDKListenersOnLB(ctx context.Context, lbARN string) ([]ListenerWithTags, error) {
	return s.taggingManager.ListListeners(ctx, lbARN)
}

type resAndSDKListenerPair struct {
	resLS *elbv2model.Listener
	sdkLS ListenerWithTags
}

func matchResAndSDKListeners(resLSs []*elbv2model.Listener, sdkLSs []ListenerWithTags) ([]resAndSDKListenerPair, []*elbv2model.Listener, []ListenerWithTags) {
	var matchedResAndSDKLSs []resAndSDKListenerPair
	var unmatchedResLSs []*elbv2model.Listener
	var unmatchedSDKLSs []ListenerWithTags

	resLSByPort := mapResListenerByPort(resLSs)
	sdkLSByPort := mapSDKListenerByPort(sdkLSs)
	resLSPorts := sets.Int64KeySet(resLSByPort)
	sdkLSPorts := sets.Int64KeySet(sdkLSByPort)
	for _, port := range resLSPorts.Intersection(sdkLSPorts).List() {
		resLS := resLSByPort[port]
		sdkLS := sdkLSByPort[port]
		matchedResAndSDKLSs = append(matchedResAndSDKLSs, resAndSDKListenerPair{
			resLS: resLS,
			sdkLS: sdkLS,
		})
	}
	for _, port := range resLSPorts.Difference(sdkLSPorts).List() {
		unmatchedResLSs = append(unmatchedResLSs, resLSByPort[port])
	}
	for _, port := range sdkLSPorts.Difference(resLSPorts).List() {
		unmatchedSDKLSs = append(unmatchedSDKLSs, sdkLSByPort[port])
	}
	return matchedResAndSDKLSs, unmatchedResLSs, unmatchedSDKLSs
}

func mapResListenerByPort(resLSs []*elbv2model.Listener) map[int64]*elbv2model.Listener {
	resLSByPort := make(map[int64]*elbv2model.Listener, len(resLSs))
	for _, ls := range resLSs {
		resLSByPort[ls.Spec.Port] = ls
	}
	return resLSByPort
}

func mapSDKListenerByPort(sdkLSs []ListenerWithTags) map[int64]ListenerWithTags {
	sdkLSByPort := make(map[int64]ListenerWithTags, len(sdkLSs))
	for _, ls := range sdkLSs {
		sdkLSByPort[awssdk.Int64Value(ls.Listener.Port)] = ls
	}
	return sdkLSByPort
}

func mapResListenerByLoadBalancerARN(resLSs []*elbv2model.Listener) (map[string][]*elbv2model.Listener, error) {
	resLSsByLBARN := make(map[string][]*elbv2model.Listener, len(resLSs))
	ctx := context.Background()
	for _, ls := range resLSs {
		lbARN, err := ls.Spec.LoadBalancerARN.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		resLSsByLBARN[lbARN] = append(resLSsByLBARN[lbARN], ls)
	}
	return resLSsByLBARN, nil
}
