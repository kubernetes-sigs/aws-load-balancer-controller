package model

import (
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/addon"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
	wafv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafv2"
	"testing"
)

func Test_buildAddons(t *testing.T) {
	lbArn := coremodel.LiteralStringToken("test")
	testCases := []struct {
		name                string
		supportedAddons     []addon.Addon
		lbCfg               elbv2gw.LoadBalancerConfiguration
		previousAddonConfig []addon.Addon
		expectedMetadata    []addon.AddonMetadata
		expectShield        bool
		shieldResult        bool
		expectWaf           bool
		wafACL              string
	}{
		{
			name:                "no supported addons",
			supportedAddons:     []addon.Addon{},
			previousAddonConfig: []addon.Addon{},
			expectedMetadata:    []addon.AddonMetadata{},
		},
		{
			name:                "no enabled addons",
			supportedAddons:     addon.AllAddons,
			previousAddonConfig: []addon.Addon{},
			expectedMetadata: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
				{
					Name:    addon.Shield,
					Enabled: false,
				},
			},
		},
		{
			name:                "enabled addon, but not supported",
			supportedAddons:     []addon.Addon{addon.WAFv2},
			previousAddonConfig: []addon.Addon{},
			expectedMetadata: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ShieldAdvanced: &elbv2gw.ShieldConfiguration{
						Enabled: true,
					},
				},
			},
		},
		{
			name:                "enabled shield",
			supportedAddons:     addon.AllAddons,
			previousAddonConfig: []addon.Addon{},
			expectedMetadata: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
				{
					Name:    addon.Shield,
					Enabled: true,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ShieldAdvanced: &elbv2gw.ShieldConfiguration{
						Enabled: true,
					},
				},
			},
			expectShield: true,
			shieldResult: true,
		},
		{
			name:                "enabled waf",
			supportedAddons:     addon.AllAddons,
			previousAddonConfig: []addon.Addon{},
			expectedMetadata: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: true,
				},
				{
					Name:    addon.Shield,
					Enabled: false,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					WAFv2: &elbv2gw.WAFv2Configuration{
						ACL: "foo",
					},
				},
			},
			expectWaf: true,
			wafACL:    "foo",
		},
		{
			name:                "waf and shield enabled",
			supportedAddons:     addon.AllAddons,
			previousAddonConfig: []addon.Addon{},
			expectedMetadata: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: true,
				},
				{
					Name:    addon.Shield,
					Enabled: true,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					WAFv2: &elbv2gw.WAFv2Configuration{
						ACL: "foo",
					},
					ShieldAdvanced: &elbv2gw.ShieldConfiguration{
						Enabled: true,
					},
				},
			},
			expectShield: true,
			shieldResult: true,
			expectWaf:    true,
			wafACL:       "foo",
		},
		{
			name:            "waf was enabled, now is not",
			supportedAddons: addon.AllAddons,
			previousAddonConfig: []addon.Addon{
				addon.WAFv2,
			},
			expectedMetadata: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
				{
					Name:    addon.Shield,
					Enabled: false,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			expectWaf: true,
			wafACL:    "",
		},
		{
			name:            "shield was enabled, now is not",
			supportedAddons: addon.AllAddons,
			previousAddonConfig: []addon.Addon{
				addon.Shield,
			},
			expectedMetadata: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
				{
					Name:    addon.Shield,
					Enabled: false,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			expectShield: true,
			shieldResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
			builder := newAddOnBuilder(logr.Discard(), tc.supportedAddons)
			metadata, err := builder.buildAddons(stack, lbArn, tc.lbCfg, tc.previousAddonConfig)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedMetadata, metadata)

			var shieldResult []*shieldmodel.Protection
			listErr := stack.ListResources(&shieldResult)
			assert.NoError(t, listErr)
			if tc.expectShield {
				assert.Equal(t, 1, len(shieldResult))
				assert.Equal(t, tc.shieldResult, shieldResult[0].Spec.Enabled)
			} else {
				assert.Equal(t, 0, len(shieldResult))
			}

			var wafV2Result []*wafv2model.WebACLAssociation
			listErr = stack.ListResources(&wafV2Result)
			assert.NoError(t, listErr)
			if tc.expectWaf {
				assert.Equal(t, 1, len(wafV2Result))
				assert.Equal(t, tc.wafACL, wafV2Result[0].Spec.WebACLARN)
			} else {
				assert.Equal(t, 0, len(wafV2Result))
			}
		})
	}
}
