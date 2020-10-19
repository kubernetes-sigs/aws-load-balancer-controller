package elbv2

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewTargetGroupBindingSynthesizer constructs new targetGroupBindingSynthesizer
func NewTargetGroupBindingSynthesizer(k8sClient client.Client, trackingProvider tracking.Provider, tgbManager TargetGroupBindingManager, logger logr.Logger, stack core.Stack) *targetGroupBindingSynthesizer {
	return &targetGroupBindingSynthesizer{
		k8sClient:        k8sClient,
		trackingProvider: trackingProvider,
		tgbManager:       tgbManager,
		logger:           logger,
		stack:            stack,

		unmatchedK8sTGBs: nil,
	}
}

// targetGroupBindingSynthesizer is responsible for synthesize TargetGroupBinding resources types for certain stack.
type targetGroupBindingSynthesizer struct {
	k8sClient        client.Client
	trackingProvider tracking.Provider
	tgbManager       TargetGroupBindingManager
	logger           logr.Logger
	stack            core.Stack

	unmatchedK8sTGBs []*elbv2api.TargetGroupBinding
}

func (s *targetGroupBindingSynthesizer) Synthesize(ctx context.Context) error {
	var resTGBs []*elbv2model.TargetGroupBindingResource
	s.stack.ListResources(&resTGBs)
	k8sTGBs, err := s.findK8sTargetGroupBindings(ctx)
	if err != nil {
		return err
	}

	matchedResAndK8sTGBs, unmatchedResTGBs, unmatchedK8sTGBs, err := matchResAndK8sTargetGroupBindings(resTGBs, k8sTGBs)
	if err != nil {
		return err
	}
	s.unmatchedK8sTGBs = unmatchedK8sTGBs

	for _, resTGB := range unmatchedResTGBs {
		tgbStatus, err := s.tgbManager.Create(ctx, resTGB)
		if err != nil {
			return err
		}
		resTGB.SetStatus(tgbStatus)
	}
	for _, resAndK8sTGB := range matchedResAndK8sTGBs {
		tgbStatus, err := s.tgbManager.Update(ctx, resAndK8sTGB.resTGB, resAndK8sTGB.k8sTGB)
		if err != nil {
			return err
		}
		resAndK8sTGB.resTGB.SetStatus(tgbStatus)
	}
	return nil
}

func (s *targetGroupBindingSynthesizer) PostSynthesize(ctx context.Context) error {
	for _, k8sTGB := range s.unmatchedK8sTGBs {
		if err := s.tgbManager.Delete(ctx, k8sTGB); err != nil {
			return err
		}
	}
	return nil
}

func (s *targetGroupBindingSynthesizer) findK8sTargetGroupBindings(ctx context.Context) ([]*elbv2api.TargetGroupBinding, error) {
	stackLabels := s.trackingProvider.StackLabels(s.stack)

	tgbList := &elbv2api.TargetGroupBindingList{}
	if err := s.k8sClient.List(ctx, tgbList, client.MatchingLabels(stackLabels)); err != nil {
		return nil, err
	}

	tgbs := make([]*elbv2api.TargetGroupBinding, 0, len(tgbList.Items))
	for i := range tgbList.Items {
		tgbs = append(tgbs, &tgbList.Items[i])
	}
	return tgbs, nil
}

type resAndK8sTargetGroupBindingPair struct {
	resTGB *elbv2model.TargetGroupBindingResource
	k8sTGB *elbv2api.TargetGroupBinding
}

func matchResAndK8sTargetGroupBindings(resTGBs []*elbv2model.TargetGroupBindingResource, k8sTGBs []*elbv2api.TargetGroupBinding) ([]resAndK8sTargetGroupBindingPair, []*elbv2model.TargetGroupBindingResource, []*elbv2api.TargetGroupBinding, error) {
	var matchedResAndK8sTGBs []resAndK8sTargetGroupBindingPair
	var unmatchedResTGBs []*elbv2model.TargetGroupBindingResource
	var unmatchedK8sTGBs []*elbv2api.TargetGroupBinding
	resTGBsByARN, err := mapResTargetGroupBindingByARN(resTGBs)
	if err != nil {
		return nil, nil, nil, err
	}
	k8sTGBsByARN := mapK8sTargetGroupBindingByARN(k8sTGBs)

	resTGBARNs := sets.StringKeySet(resTGBsByARN)
	k8sTGBARNs := sets.StringKeySet(k8sTGBsByARN)
	for _, tgARN := range resTGBARNs.Intersection(k8sTGBARNs).List() {
		resTGB := resTGBsByARN[tgARN]
		k8sTGB := k8sTGBsByARN[tgARN]
		matchedResAndK8sTGBs = append(matchedResAndK8sTGBs, resAndK8sTargetGroupBindingPair{
			resTGB: resTGB,
			k8sTGB: k8sTGB,
		})
	}

	for _, tgARN := range resTGBARNs.Difference(k8sTGBARNs).List() {
		unmatchedResTGBs = append(unmatchedResTGBs, resTGBsByARN[tgARN])
	}
	for _, tgARN := range k8sTGBARNs.Difference(resTGBARNs).List() {
		unmatchedK8sTGBs = append(unmatchedK8sTGBs, k8sTGBsByARN[tgARN])
	}

	return matchedResAndK8sTGBs, unmatchedResTGBs, unmatchedK8sTGBs, nil
}

func mapResTargetGroupBindingByARN(resTGBs []*elbv2model.TargetGroupBindingResource) (map[string]*elbv2model.TargetGroupBindingResource, error) {
	ctx := context.Background()
	resTGBsByARN := make(map[string]*elbv2model.TargetGroupBindingResource, len(resTGBs))
	for _, resTGB := range resTGBs {
		tgARN, err := resTGB.Spec.Template.Spec.TargetGroupARN.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		resTGBsByARN[tgARN] = resTGB
	}
	return resTGBsByARN, nil
}

func mapK8sTargetGroupBindingByARN(k8sTGBs []*elbv2api.TargetGroupBinding) map[string]*elbv2api.TargetGroupBinding {
	k8sTGBsByARN := make(map[string]*elbv2api.TargetGroupBinding, len(k8sTGBs))
	for _, k8sTGB := range k8sTGBs {
		tgARN := k8sTGB.Spec.TargetGroupARN
		k8sTGBsByARN[tgARN] = k8sTGB
	}
	return k8sTGBsByARN
}
