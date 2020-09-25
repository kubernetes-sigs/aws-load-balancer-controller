package shield

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
)

const (
	protectionNameManaged       = "managed by aws-load-balancer-controller"
	protectionNameManagedLegacy = "managed by aws-alb-ingress-controller"
)

// NewProtectionSynthesizer constructs new protectionSynthesizer
func NewProtectionSynthesizer(protectionManager ProtectionManager, logger logr.Logger, stack core.Stack) *protectionSynthesizer {
	return &protectionSynthesizer{
		protectionManager: protectionManager,
		logger:            logger,
		stack:             stack,
	}
}

type protectionSynthesizer struct {
	protectionManager ProtectionManager
	logger            logr.Logger
	stack             core.Stack
}

func (s *protectionSynthesizer) Synthesize(ctx context.Context) error {
	var resProtections []*shieldmodel.Protection
	s.stack.ListResources(&resProtections)
	resProtectionsByResARN, err := mapResProtectionByResourceARN(resProtections)
	if err != nil {
		return err
	}

	var resLBs []*elbv2model.LoadBalancer
	s.stack.ListResources(&resLBs)
	for _, resLB := range resLBs {
		// shield protection can only be associated with ALB for now.
		if resLB.Spec.Type != elbv2model.LoadBalancerTypeApplication {
			continue
		}
		lbARN, err := resLB.LoadBalancerARN().Resolve(ctx)
		if err != nil {
			return err
		}
		resProtections := resProtectionsByResARN[lbARN]
		if err := s.synthesizeProtectionsOnLB(ctx, lbARN, resProtections); err != nil {
			return err
		}
	}
	return nil
}

func (s *protectionSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here.
	return nil
}

func (s *protectionSynthesizer) synthesizeProtectionsOnLB(ctx context.Context, lbARN string, resProtections []*shieldmodel.Protection) error {
	if len(resProtections) > 1 {
		return errors.Errorf("[should never happen] multiple shield protection desired on LoadBalancer: %v", lbARN)
	}

	enableProtection := false
	if len(resProtections) == 1 {
		enableProtection = true
	}

	protectionInfo, err := s.protectionManager.GetProtection(ctx, lbARN)
	if err != nil {
		return err
	}
	switch {
	case !enableProtection && protectionInfo != nil:
		managedProtectionNames := sets.NewString(protectionNameManaged, protectionNameManagedLegacy)
		if managedProtectionNames.Has(protectionInfo.Name) {
			if err := s.protectionManager.DeleteProtection(ctx, lbARN, protectionInfo.ID); err != nil {
				return errors.Wrap(err, "failed to delete shield protection on LoadBalancer")
			}
		} else {
			s.logger.Info("ignoring unmanaged shield protection",
				"protectionName", protectionInfo.Name,
				"protectionID", protectionInfo.ID)
		}
	case enableProtection && protectionInfo == nil:
		if _, err := s.protectionManager.CreateProtection(ctx, lbARN, protectionNameManaged); err != nil {
			return errors.Wrap(err, "failed to create shield protection on LoadBalancer")
		}
	}
	return nil
}

func mapResProtectionByResourceARN(resProtections []*shieldmodel.Protection) (map[string][]*shieldmodel.Protection, error) {
	resProtectionsByResARN := make(map[string][]*shieldmodel.Protection, len(resProtections))
	ctx := context.Background()
	for _, resProtection := range resProtections {
		resARN, err := resProtection.Spec.ResourceARN.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		resProtectionsByResARN[resARN] = append(resProtectionsByResARN[resARN], resProtection)
	}
	return resProtectionsByResARN, nil
}
