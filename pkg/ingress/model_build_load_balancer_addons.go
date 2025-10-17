package ingress

import (
	"context"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
	wafregionalmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafregional"
	wafv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

const (
	// sentinel annotation value to disable wafv2 ACL on resources.
	wafv2ACLARNNone = "none"
	// sentinel annotation value to disable wafRegional on resources.
	webACLIDNone = "none"
)

func (t *defaultModelBuildTask) buildLoadBalancerAddOns(ctx context.Context, lbARN core.StringToken) error {
	if _, err := t.buildWAFv2WebACLAssociation(ctx, lbARN); err != nil {
		return err
	}
	if _, err := t.buildWAFRegionalWebACLAssociation(ctx, lbARN); err != nil {
		return err
	}
	if _, err := t.buildShieldProtection(ctx, lbARN); err != nil {
		return err
	}
	return nil
}

func (t *defaultModelBuildTask) buildWAFv2WebACLAssociation(ctx context.Context, lbARN core.StringToken) (*wafv2model.WebACLAssociation, error) {
	explicitWebACLARNs := sets.NewString()
	explicitWebACLNames := sets.NewString()

	for _, member := range t.ingGroup.Members {
		if member.IngClassConfig.IngClassParams != nil && member.IngClassConfig.IngClassParams.Spec.WAFv2ACLName != "" {
			rawWebACLName := member.IngClassConfig.IngClassParams.Spec.WAFv2ACLName
			explicitWebACLNames.Insert(rawWebACLName)
			continue
		}
		rawWebACLName := ""
		_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixWAFv2ACLName, &rawWebACLName, member.Ing.Annotations)
		if rawWebACLName != "" {
			explicitWebACLNames.Insert(rawWebACLName)
		}
	}

	webACLARN := ""

	if len(explicitWebACLNames) == 0 {
		for _, member := range t.ingGroup.Members {
			if member.IngClassConfig.IngClassParams != nil && member.IngClassConfig.IngClassParams.Spec.WAFv2ACLArn != "" {
				webACLARN = member.IngClassConfig.IngClassParams.Spec.WAFv2ACLArn
				explicitWebACLARNs.Insert(webACLARN)
				continue
			}

			rawWebACLARN := ""
			if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixWAFv2ACLARN, &rawWebACLARN, member.Ing.Annotations); !exists {
				continue
			}
			explicitWebACLARNs.Insert(rawWebACLARN)
		}
		if len(explicitWebACLARNs) == 0 {
			return nil, nil
		}
		if len(explicitWebACLARNs) > 1 {
			return nil, errors.Errorf("conflicting WAFv2 WebACL ARNs: %v", explicitWebACLARNs.List())
		}
		webACLARN, _ = explicitWebACLARNs.PopAny()
	}

	if len(explicitWebACLNames) > 1 {
		return nil, errors.Errorf("conflicting WAFv2 WebACL names: %v", explicitWebACLNames.List())
	}

	if len(explicitWebACLNames) == 1 {
		rawWebACLName, _ := explicitWebACLNames.PopAny()
		if rawWebACLName != "none" {
			var err error
			webACLARN, err = t.webACLNameToArnMapper.getArnByName(ctx, rawWebACLName)
			if err != nil {
				return nil, errors.Errorf("couldn't find WAFv2 WebACL with name: %v", rawWebACLName)
			}
		}
	}

	switch webACLARN {
	case wafv2ACLARNNone:
		association := wafv2model.NewWebACLAssociation(t.stack, shared_constants.ResourceIDLoadBalancer, wafv2model.WebACLAssociationSpec{
			WebACLARN:   "",
			ResourceARN: lbARN,
		})
		return association, nil
	default:
		association := wafv2model.NewWebACLAssociation(t.stack, shared_constants.ResourceIDLoadBalancer, wafv2model.WebACLAssociationSpec{
			WebACLARN:   webACLARN,
			ResourceARN: lbARN,
		})
		return association, nil
	}
}

func (t *defaultModelBuildTask) buildWAFRegionalWebACLAssociation(_ context.Context, lbARN core.StringToken) (*wafregionalmodel.WebACLAssociation, error) {
	explicitWebACLIDs := sets.NewString()
	for _, member := range t.ingGroup.Members {
		rawWebACLID := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixWAFACLID, &rawWebACLID, member.Ing.Annotations); !exists {
			_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixWebACLID, &rawWebACLID, member.Ing.Annotations)
		}
		if rawWebACLID != "" {
			explicitWebACLIDs.Insert(rawWebACLID)
		}
	}
	if len(explicitWebACLIDs) == 0 {
		return nil, nil
	}
	if len(explicitWebACLIDs) > 1 {
		return nil, errors.Errorf("conflicting WAFClassic WebACL IDs: %v", explicitWebACLIDs.List())
	}
	webACLID, _ := explicitWebACLIDs.PopAny()
	switch webACLID {
	case webACLIDNone:
		association := wafregionalmodel.NewWebACLAssociation(t.stack, shared_constants.ResourceIDLoadBalancer, wafregionalmodel.WebACLAssociationSpec{
			WebACLID:    "",
			ResourceARN: lbARN,
		})
		return association, nil
	default:
		association := wafregionalmodel.NewWebACLAssociation(t.stack, shared_constants.ResourceIDLoadBalancer, wafregionalmodel.WebACLAssociationSpec{
			WebACLID:    webACLID,
			ResourceARN: lbARN,
		})
		return association, nil
	}
}

func (t *defaultModelBuildTask) buildShieldProtection(_ context.Context, lbARN core.StringToken) (*shieldmodel.Protection, error) {
	explicitEnableProtections := make(map[bool]struct{})
	for _, member := range t.ingGroup.Members {
		rawEnableProtection := false
		exists, err := t.annotationParser.ParseBoolAnnotation(annotations.IngressSuffixShieldAdvancedProtection, &rawEnableProtection, member.Ing.Annotations)
		if err != nil {
			return nil, err
		}
		if exists {
			explicitEnableProtections[rawEnableProtection] = struct{}{}
		}
	}
	if len(explicitEnableProtections) == 0 {
		return nil, nil
	}
	if len(explicitEnableProtections) > 1 {
		return nil, errors.New("conflicting enable shield advanced protection")
	}
	_, enableProtection := explicitEnableProtections[true]
	protection := shieldmodel.NewProtection(t.stack, shared_constants.ResourceIDLoadBalancer, shieldmodel.ProtectionSpec{
		Enabled:     enableProtection,
		ResourceARN: lbARN,
	})
	return protection, nil
}
