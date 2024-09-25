package wafregional

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
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

func (s *webACLAssociationSynthesizer) synthesizeWebACLAssociationsOnLB(ctx context.Context, lbARN string, resAssociations []*wafregionalmodel.WebACLAssociation) error {
	if len(resAssociations) != 1 {
		return errors.Errorf("[should never happen] should be exactly one WAFClassic webACL desired on LoadBalancer: %v", lbARN)
	}
	desiredWebACLID := resAssociations[0].Spec.WebACLID
	currentWebACLID, err := s.associationManager.GetAssociatedWebACL(ctx, lbARN)
	if err != nil {
		return errors.Wrap(err, "failed to get WAFClassic webACL association on LoadBalancer")
	}
	switch {
	case desiredWebACLID == "" && currentWebACLID != "":
		if err := s.associationManager.DisassociateWebACL(ctx, lbARN); err != nil {
			return errors.Wrap(err, "failed to delete WAFClassic webACL association on LoadBalancer")
		}
	case desiredWebACLID != "" && currentWebACLID == "":
		if err := s.associationManager.AssociateWebACL(ctx, lbARN, desiredWebACLID); err != nil {
			return errors.Wrap(err, "failed to create WAFClassic webACL association on LoadBalancer")
		}
	case desiredWebACLID != "" && desiredWebACLID != currentWebACLID:
		if err := s.associationManager.AssociateWebACL(ctx, lbARN, desiredWebACLID); err != nil {
			return errors.Wrap(err, "failed to update WAFClassic webACL association on LoadBalancer")
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
