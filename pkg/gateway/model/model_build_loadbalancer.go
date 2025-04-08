package model

import (
	"context"
	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type loadBalancerBuilder interface {
	buildLoadBalancerSpec(ctx context.Context, gw *gwv1.Gateway, stack core.Stack, lbConf *elbv2gw.LoadBalancerConfiguration, routes map[int][]routeutils.RouteDescriptor) (elbv2model.LoadBalancerSpec, error)
}

type loadBalancerBuilderImpl struct {
	subnetBuilder subnetModelBuilder

	defaultLoadBalancerScheme elbv2model.LoadBalancerScheme
	defaultIPType             elbv2model.IPAddressType
}

func newLoadBalancerBuilder(subnetBuilder subnetModelBuilder, defaultLoadBalancerScheme string) loadBalancerBuilder {

	return &loadBalancerBuilderImpl{
		subnetBuilder:             subnetBuilder,
		defaultLoadBalancerScheme: elbv2model.LoadBalancerScheme(defaultLoadBalancerScheme),
		defaultIPType:             elbv2model.IPAddressTypeIPV4,
	}
}

func (lbModelBuilder *loadBalancerBuilderImpl) buildLoadBalancerSpec(ctx context.Context, gw *gwv1.Gateway, stack core.Stack, lbConf *elbv2gw.LoadBalancerConfiguration, routes map[int][]routeutils.RouteDescriptor) (elbv2model.LoadBalancerSpec, error) {
	scheme, err := lbModelBuilder.buildLoadBalancerScheme(lbConf)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	ipAddressType, err := lbModelBuilder.buildLoadBalancerIPAddressType(lbConf)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	configuredSubnets, sourcePrefixEnabled, err := lbModelBuilder.subnetBuilder.buildLoadBalancerSubnets(ctx, lbConf.Spec.LoadBalancerSubnets, lbConf.Spec.LoadBalancerSubnetsSelector, scheme, ipAddressType, stack)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}

	return elbv2model.LoadBalancerSpec{
		Type:                         elbv2model.LoadBalancerTypeApplication,
		Scheme:                       scheme,
		IPAddressType:                ipAddressType,
		SubnetMappings:               configuredSubnets,
		EnablePrefixForIpv6SourceNat: lbModelBuilder.translateSourcePrefixEnabled(sourcePrefixEnabled),
	}, nil
}

func (lbModelBuilder *loadBalancerBuilderImpl) translateSourcePrefixEnabled(b bool) elbv2model.EnablePrefixForIpv6SourceNat {
	if b {
		return elbv2model.EnablePrefixForIpv6SourceNatOn
	}
	return elbv2model.EnablePrefixForIpv6SourceNatOff

}

func (lbModelBuilder *loadBalancerBuilderImpl) buildLoadBalancerScheme(lbConf *elbv2gw.LoadBalancerConfiguration) (elbv2model.LoadBalancerScheme, error) {
	scheme := lbConf.Spec.Scheme

	if scheme == nil {
		return lbModelBuilder.defaultLoadBalancerScheme, nil
	}
	switch *scheme {
	case elbv2gw.LoadBalancerScheme(elbv2model.LoadBalancerSchemeInternetFacing):
		return elbv2model.LoadBalancerSchemeInternetFacing, nil
	case elbv2gw.LoadBalancerScheme(elbv2model.LoadBalancerSchemeInternal):
		return elbv2model.LoadBalancerSchemeInternal, nil
	default:
		return "", errors.Errorf("unknown scheme: %v", *scheme)
	}
}

// buildLoadBalancerIPAddressType builds the LoadBalancer IPAddressType.
func (lbModelBuilder *loadBalancerBuilderImpl) buildLoadBalancerIPAddressType(lbConf *elbv2gw.LoadBalancerConfiguration) (elbv2model.IPAddressType, error) {

	if lbConf.Spec.IpAddressType == nil {
		return lbModelBuilder.defaultIPType, nil
	}

	switch *lbConf.Spec.IpAddressType {
	case elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeIPV4):
		return elbv2model.IPAddressTypeIPV4, nil
	case elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeDualStack):
		return elbv2model.IPAddressTypeDualStack, nil
	case elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeDualStackWithoutPublicIPV4):
		return elbv2model.IPAddressTypeDualStackWithoutPublicIPV4, nil
	default:
		return "", errors.Errorf("unknown IPAddressType: %v", *lbConf.Spec.IpAddressType)
	}
}
