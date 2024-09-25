package wafv2

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
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
	if err := s.stack.ListResources(&resAssociations); err != nil {
		return fmt.Errorf("[should never happen] failed to list resources: %w", err)
	}
	if len(resAssociations) == 0 {
		return nil
	}
	resAssociationsByResARN, err := mapResWebACLAssociationByResourceARN(resAssociations)
	if err != nil {
		return err
	}
	for resARN, webACLAssociations := range resAssociationsByResARN {
		if err := s.synthesizeWebACLAssociationsOnLB(ctx, resARN, webACLAssociations); err != nil {
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
	if len(resAssociations) != 1 {
		return errors.Errorf("[should never happen] should be exactly one WAFv2 webACL association on LoadBalancer: %v", lbARN)
	}
	desiredWebACLARN := resAssociations[0].Spec.WebACLARN
	currentWebACLARN, err := s.associationManager.GetAssociatedWebACL(ctx, lbARN)
	if err != nil {
		return errors.Wrap(err, "failed to get WAFv2 webACL association on LoadBalancer")
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
	case desiredWebACLARN != "" && desiredWebACLARN != currentWebACLARN:
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
