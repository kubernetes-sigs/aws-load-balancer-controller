package aga

import (
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
)

// GlobalAcceleratorTagProvider adapts GlobalAccelerator to implement TagProvider
type GlobalAcceleratorTagProvider struct {
	ga agaapi.GlobalAccelerator
}

// NewGlobalAcceleratorTagProvider creates a new GlobalAcceleratorTagProvider
func NewGlobalAcceleratorTagProvider(ga agaapi.GlobalAccelerator) shared_utils.TagProvider {
	return &GlobalAcceleratorTagProvider{ga: ga}
}

// GetTags returns the tags from the GlobalAccelerator spec
func (g GlobalAcceleratorTagProvider) GetTags() *map[string]string {
	return g.ga.Spec.Tags
}
