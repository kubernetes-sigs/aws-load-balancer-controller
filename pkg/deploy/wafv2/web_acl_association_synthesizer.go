package wafv2

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	wafv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafv2"
)

// NewWebACLAssociationSynthesizer constructs new webACLAssociationSynthesizer.
func NewWebACLAssociationSynthesizer(associationManager WebACLAssociationManager, logger logr.Logger, stack core.Stack) *webACLAssociationSynthesizer {
	return &webACLAssociationSynthesizer{
		associationManager: associationManager,
		logger:             logger,
		stack:              stack,
	}
}

type webACLAssociationSynthesizer struct {
	associationManager WebACLAssociationManager
	logger             logr.Logger
	stack              core.Stack
}

func (s *webACLAssociationSynthesizer) Synthesize(ctx context.Context) error {
	var resAssociations []*wafv2model.WebACLAssociation
	s.stack.ListResources(&resAssociations)
	resAssociationsByResARN, err := mapResWebACLAssociationByResourceARN(resAssociations)
	if err != nil {
		return err
	}

	var resLBs []*elbv2model.LoadBalancer
	s.stack.ListResources(&resLBs)
	for _, resLB := range resLBs {
		// wafv2 WebACL can only be associated with ALB for now.
		if resLB.Spec.Type != elbv2model.LoadBalancerTypeApplication {
			continue
		}
		lbARN, err := resLB.LoadBalancerARN().Resolve(ctx)
		if err != nil {
			return err
		}
		resAssociations := resAssociationsByResARN[lbARN]
		if err := s.synthesizeWebACLAssociationsOnLB(ctx, lbARN, resAssociations); err != nil {
			return err
		}
	}
	return nil
}

func (s *webACLAssociationSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here.
	return nil
}

func (s *webACLAssociationSynthesizer) synthesizeWebACLAssociationsOnLB(ctx context.Context, lbARN string, resAssociations []*wafv2model.WebACLAssociation) error {
	if len(resAssociations) > 1 {
		return errors.Errorf("[should never happen] multiple WAFv2 webACL desired on LoadBalancer: %v", lbARN)
	}

	var desiredWebACLARN string
	if len(resAssociations) == 1 {
		desiredWebACLARN = resAssociations[0].Spec.WebACLARN
	}
	currentWebACLARN, err := s.associationManager.GetAssociatedWebACL(ctx, lbARN)
	if err != nil {
		return err
	}
	switch {
	case desiredWebACLARN == "" && currentWebACLARN != "":
		if err := s.associationManager.DisassociateWebACL(ctx, lbARN); err != nil {
			return errors.Wrap(err, "failed to delete WAFv2 webACL association on LoadBalancer")
		}
	case desiredWebACLARN != "" && currentWebACLARN == "":
		if err := s.associationManager.AssociateWebACL(ctx, lbARN, desiredWebACLARN); err != nil {
			return errors.Wrap(err, "failed to create WAFv2 webACL association on LoadBalancer")
		}
	case desiredWebACLARN != "" && currentWebACLARN != "" && desiredWebACLARN != currentWebACLARN:
		if err := s.associationManager.AssociateWebACL(ctx, lbARN, desiredWebACLARN); err != nil {
			return errors.Wrap(err, "failed to update WAFv2 webACL association on LoadBalancer")
		}
	}
	return nil
}

func mapResWebACLAssociationByResourceARN(resAssociations []*wafv2model.WebACLAssociation) (map[string][]*wafv2model.WebACLAssociation, error) {
	resAssociationsByResARN := make(map[string][]*wafv2model.WebACLAssociation, len(resAssociations))
	ctx := context.Background()
	for _, resAssociation := range resAssociations {
		resARN, err := resAssociation.Spec.ResourceARN.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		resAssociationsByResARN[resARN] = append(resAssociationsByResARN[resARN], resAssociation)
	}
	return resAssociationsByResARN, nil
}
