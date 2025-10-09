package addons

import (
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/addon"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

type PreStackAddon interface {
	AddToStack(stack core.Stack, lbARN core.StringToken)
}

type AddOnBuilder interface {
	BuildAddons(lbSpec *elbv2model.LoadBalancerSpec, lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) ([]addon.AddonMetadata, []PreStackAddon, error)
}

var _ AddOnBuilder = &addOnBuilderImpl{}

type addOnBuilderImpl struct {
	supportedAddons []addon.Addon
	logger          logr.Logger
}

func NewAddOnBuilder(logger logr.Logger, supportedAddons []addon.Addon) AddOnBuilder {
	return &addOnBuilderImpl{logger: logger, supportedAddons: supportedAddons}
}

func (aob *addOnBuilderImpl) BuildAddons(lbSpec *elbv2model.LoadBalancerSpec, lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) ([]addon.AddonMetadata, []PreStackAddon, error) {
	metadata := make([]addon.AddonMetadata, 0)
	prestack := make([]PreStackAddon, 0)
	for _, supportedAddon := range aob.supportedAddons {
		switch supportedAddon {
		case addon.WAFv2:
			wafEnabled, wafPrestack := aob.buildWAFv2(lbCfg, previousAddonConfig)
			metadata = append(metadata, addon.AddonMetadata{
				Name:    addon.WAFv2,
				Enabled: wafEnabled,
			})
			prestack = append(prestack, wafPrestack)
			break
		case addon.Shield:
			shieldEnabled, shieldPrestack := aob.buildShield(lbCfg, previousAddonConfig)
			metadata = append(metadata, addon.AddonMetadata{
				Name:    addon.Shield,
				Enabled: shieldEnabled,
			})
			prestack = append(prestack, shieldPrestack)
			break
		case addon.ProvisionedCapacity:
			// PC doesn't rely on the resource stack, as it's added directly to the LB Spec hence no prestack addon needed.
			metadata = append(metadata, addon.AddonMetadata{
				Name:    addon.ProvisionedCapacity,
				Enabled: aob.buildProvisionedCapacity(lbSpec, lbCfg, previousAddonConfig),
			})
		default:
			return nil, nil, errors.Errorf("Unknown addon %s", supportedAddon)
		}
	}
	return metadata, prestack, nil
}

func (aob *addOnBuilderImpl) buildWAFv2(lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) (bool, PreStackAddon) {
	var webACLARN string
	if lbCfg.Spec.WAFv2 != nil {
		webACLARN = lbCfg.Spec.WAFv2.ACL
	}

	// Check if we're trying to disable the WAF ACL, if so, we should only do so if the addon is active.
	// We should not call WAF repeatedly when the user has disabled it.
	if webACLARN == "" && !aob.isAddonActive(addon.WAFv2, previousAddonConfig) {
		return false, makeNoOpPrestack()
	}

	return webACLARN != "", makeWAFPrestack(webACLARN)
}

func (aob *addOnBuilderImpl) buildShield(lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) (bool, PreStackAddon) {
	var shieldEnabled bool
	if lbCfg.Spec.ShieldAdvanced != nil {
		shieldEnabled = lbCfg.Spec.ShieldAdvanced.Enabled
	}

	// Check if we're trying to disable Shield, if so, we should only do so if the addon is active.
	// We should not call Shield repeatedly when the user has disabled it.
	if !shieldEnabled && !aob.isAddonActive(addon.Shield, previousAddonConfig) {
		return false, makeNoOpPrestack()
	}

	return shieldEnabled, makeShieldPrestack(shieldEnabled)
}

func (aob *addOnBuilderImpl) buildProvisionedCapacity(lbSpec *elbv2model.LoadBalancerSpec, lbCfg elbv2gw.LoadBalancerConfiguration, previousAddonConfig []addon.Addon) bool {
	var minCapacity int32
	if lbCfg.Spec.MinimumLoadBalancerCapacity != nil {
		minCapacity = lbCfg.Spec.MinimumLoadBalancerCapacity.CapacityUnits
	}

	// Check if we're trying to disable PC, if so, we should only do so if the addon is active.
	// We should not call PC APIs repeatedly when the user has disabled it.
	if minCapacity == 0 && !aob.isAddonActive(addon.ProvisionedCapacity, previousAddonConfig) {
		return false
	}

	lbSpec.MinimumLoadBalancerCapacity = &elbv2model.MinimumLoadBalancerCapacity{
		CapacityUnits: minCapacity,
	}

	return minCapacity != 0
}

func (aob *addOnBuilderImpl) isAddonActive(target addon.Addon, previousAddonConfig []addon.Addon) bool {
	for _, prev := range previousAddonConfig {
		if prev == target {
			return true
		}
	}
	return false
}
