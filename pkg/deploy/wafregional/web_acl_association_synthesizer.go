package wafregional

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	wafregionalmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafregional"
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
	var resAssociations []*wafregionalmodel.WebACLAssociation
	s.stack.ListResources(&resAssociations)
	resAssociationsByResARN, err := mapResWebACLAssociationByResourceARN(resAssociations)
	if err != nil {
		return err
	}

	var resLBs []*elbv2model.LoadBalancer
	s.stack.ListResources(&resLBs)
	for _, resLB := range resLBs {
		// wafRegional WebACL can only be associated with ALB for now.
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

func (s *webACLAssociationSynthesizer) synthesizeWebACLAssociationsOnLB(ctx context.Context, lbARN string, resAssociations []*wafregionalmodel.WebACLAssociation) error {
	if len(resAssociations) > 1 {
		return errors.Errorf("[should never happen] multiple WAFRegional webACL desired on LoadBalancer: %v", lbARN)
	}

	var desiredWebACLID string
	if len(resAssociations) == 1 {
		desiredWebACLID = resAssociations[0].Spec.WebACLID
	}
	currentWebACLID, err := s.associationManager.GetAssociatedWebACL(ctx, lbARN)
	if err != nil {
		return err
	}
	switch {
	case desiredWebACLID == "" && currentWebACLID != "":
		if err := s.associationManager.DisassociateWebACL(ctx, lbARN); err != nil {
			return errors.Wrap(err, "failed to delete WAFv2 WAFRegional association on LoadBalancer")
		}
	case desiredWebACLID != "" && currentWebACLID == "":
		if err := s.associationManager.AssociateWebACL(ctx, lbARN, desiredWebACLID); err != nil {
			return errors.Wrap(err, "failed to create WAFv2 WAFRegional association on LoadBalancer")
		}
	case desiredWebACLID != "" && currentWebACLID != "" && desiredWebACLID != currentWebACLID:
		if err := s.associationManager.AssociateWebACL(ctx, lbARN, desiredWebACLID); err != nil {
			return errors.Wrap(err, "failed to update WAFv2 WAFRegional association on LoadBalancer")
		}
	}
	return nil
}

func mapResWebACLAssociationByResourceARN(resAssociations []*wafregionalmodel.WebACLAssociation) (map[string][]*wafregionalmodel.WebACLAssociation, error) {
	resAssociationsByResARN := make(map[string][]*wafregionalmodel.WebACLAssociation, len(resAssociations))
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
