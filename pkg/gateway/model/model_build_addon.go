package model

import (
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/addon"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
	wafv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafv2"
)

type addOnBuilder interface {
	buildAddons(stack core.Stack, lbARN core.StringToken, lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) ([]addon.AddonMetadata, error)
}

var _ addOnBuilder = &addOnBuilderImpl{}

type addOnBuilderImpl struct {
	supportedAddons []addon.Addon
	logger          logr.Logger
}

func newAddOnBuilder(logger logr.Logger, supportedAddons []addon.Addon) addOnBuilder {
	return &addOnBuilderImpl{logger: logger, supportedAddons: supportedAddons}
}

func (aob *addOnBuilderImpl) buildAddons(stack core.Stack, lbARN core.StringToken, lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) ([]addon.AddonMetadata, error) {
	result := make([]addon.AddonMetadata, 0)
	for _, supportedAddon := range aob.supportedAddons {
		switch supportedAddon {
		case addon.WAFv2:
			wafEnabled, err := aob.buildWAFv2(stack, lbARN, lbCfg, previousAddonConfig)
			if err != nil {
				return nil, err
			}
			result = append(result, addon.AddonMetadata{
				Name:    addon.WAFv2,
				Enabled: wafEnabled,
			})
			break
		case addon.Shield:
			shieldEnabled, err := aob.buildShield(stack, lbARN, lbCfg, previousAddonConfig)
			if err != nil {
				return nil, err
			}
			result = append(result, addon.AddonMetadata{
				Name:    addon.Shield,
				Enabled: shieldEnabled,
			})
			break
		default:
			return nil, errors.Errorf("Unknown addon %s", supportedAddon)
		}
	}
	return result, nil
}

func (aob *addOnBuilderImpl) buildWAFv2(stack core.Stack, lbARN core.StringToken, lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) (bool, error) {
	var webACLARN string
	if lbCfg.Spec.WAFv2 != nil {
		webACLARN = lbCfg.Spec.WAFv2.ACL
	}

	aob.logger.Info("WAF ACL", "ACL", webACLARN)

	// Check if we're trying to disable the WAF ACL, if so, we should only do so if the addon is active.
	// We should not call WAF repeatedly when the user has disabled it.
	if webACLARN == "" && !aob.isAddonActive(addon.WAFv2, previousAddonConfig) {
		return false, nil
	}

	wafv2model.NewWebACLAssociation(stack, resourceIDLoadBalancer, wafv2model.WebACLAssociationSpec{
		WebACLARN:   webACLARN,
		ResourceARN: lbARN,
	})

	return webACLARN != "", nil
}

func (aob *addOnBuilderImpl) buildShield(stack core.Stack, lbARN core.StringToken, lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) (bool, error) {
	var shieldEnabled bool
	if lbCfg.Spec.ShieldAdvanced != nil {
		shieldEnabled = lbCfg.Spec.ShieldAdvanced.Enabled
	}

	// Check if we're trying to disable Shield, if so, we should only do so if the addon is active.
	// We should not call Shield repeatedly when the user has disabled it.
	if !shieldEnabled && !aob.isAddonActive(addon.Shield, previousAddonConfig) {
		return false, nil
	}

	shieldmodel.NewProtection(stack, resourceIDLoadBalancer, shieldmodel.ProtectionSpec{
		Enabled:     shieldEnabled,
		ResourceARN: lbARN,
	})

	return shieldEnabled, nil
}

func (aob *addOnBuilderImpl) isAddonActive(target addon.Addon, previousAddonConfig []addon.Addon) bool {
	for _, prev := range previousAddonConfig {
		if prev == target {
			return true
		}
	}
	return false
}
