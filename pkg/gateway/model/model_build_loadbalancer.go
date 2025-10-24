package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var invalidLoadBalancerNamePattern = regexp.MustCompile("[[:^alnum:]]")

type loadBalancerBuilder interface {
	buildLoadBalancerSpec(scheme elbv2model.LoadBalancerScheme, ipAddressType elbv2model.IPAddressType, gw *gwv1.Gateway, lbConf elbv2gw.LoadBalancerConfiguration, subnets buildLoadBalancerSubnetsOutput, securityGroupTokens []core.StringToken) (elbv2model.LoadBalancerSpec, error)
}

type loadBalancerBuilderImpl struct {
	loadBalancerType elbv2model.LoadBalancerType
	clusterName      string
	tagHelper        tagHelper
}

func newLoadBalancerBuilder(loadBalancerType elbv2model.LoadBalancerType, tagHelper tagHelper, clusterName string) loadBalancerBuilder {
	return &loadBalancerBuilderImpl{
		loadBalancerType: loadBalancerType,
		clusterName:      clusterName,
		tagHelper:        tagHelper,
	}
}

func (lbModelBuilder *loadBalancerBuilderImpl) buildLoadBalancerSpec(scheme elbv2model.LoadBalancerScheme, ipAddressType elbv2model.IPAddressType, gw *gwv1.Gateway, lbConf elbv2gw.LoadBalancerConfiguration, subnets buildLoadBalancerSubnetsOutput, securityGroupTokens []core.StringToken) (elbv2model.LoadBalancerSpec, error) {

	name, err := lbModelBuilder.buildLoadBalancerName(lbConf, gw, scheme)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}

	tags, err := lbModelBuilder.tagHelper.getLoadBalancerTags(lbConf)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}

	spec := elbv2model.LoadBalancerSpec{
		Name:                   name,
		Type:                   lbModelBuilder.loadBalancerType,
		Scheme:                 scheme,
		IPAddressType:          ipAddressType,
		SubnetMappings:         subnets.subnets,
		SecurityGroups:         securityGroupTokens,
		LoadBalancerAttributes: lbModelBuilder.buildLoadBalancerAttributes(lbConf),
		Tags:                   tags,
	}

	if lbModelBuilder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
		lbModelBuilder.addL4Fields(&spec, lbConf, subnets)
	}

	if lbModelBuilder.loadBalancerType == elbv2model.LoadBalancerTypeApplication {
		lbModelBuilder.addL7Fields(&spec, lbConf)
	}

	return spec, nil
}

func (lbModelBuilder *loadBalancerBuilderImpl) addL4Fields(spec *elbv2model.LoadBalancerSpec, lbConf elbv2gw.LoadBalancerConfiguration, subnets buildLoadBalancerSubnetsOutput) {
	spec.EnablePrefixForIpv6SourceNat = lbModelBuilder.translateSourcePrefixEnabled(subnets.sourceIPv6NatEnabled)

	if lbConf.Spec.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic != nil {
		spec.SecurityGroupsInboundRulesOnPrivateLink = (*elbv2model.SecurityGroupsInboundRulesOnPrivateLinkStatus)(lbConf.Spec.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic)
	}
}

func (lbModelBuilder *loadBalancerBuilderImpl) addL7Fields(spec *elbv2model.LoadBalancerSpec, lbConf elbv2gw.LoadBalancerConfiguration) {
	spec.CustomerOwnedIPv4Pool = lbConf.Spec.CustomerOwnedIpv4Pool
	spec.IPv4IPAMPool = lbConf.Spec.IPv4IPAMPoolId
}

func (lbModelBuilder *loadBalancerBuilderImpl) translateSourcePrefixEnabled(sourceNATEnabled bool) elbv2model.EnablePrefixForIpv6SourceNat {
	if sourceNATEnabled {
		return elbv2model.EnablePrefixForIpv6SourceNatOn
	}
	return elbv2model.EnablePrefixForIpv6SourceNatOff
}

func (lbModelBuilder *loadBalancerBuilderImpl) buildLoadBalancerName(lbConf elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, scheme elbv2model.LoadBalancerScheme) (string, error) {
	if lbConf.Spec.LoadBalancerName != nil {
		name := *lbConf.Spec.LoadBalancerName
		return name, nil
	}
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(lbModelBuilder.clusterName))
	_, _ = uuidHash.Write([]byte(gw.UID))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidLoadBalancerNamePattern.ReplaceAllString(gw.Namespace, "")
	sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(gw.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid), nil
}

func (lbModelBuilder *loadBalancerBuilderImpl) buildLoadBalancerAttributes(lbConf elbv2gw.LoadBalancerConfiguration) []elbv2model.LoadBalancerAttribute {
	var attributes []elbv2model.LoadBalancerAttribute
	for _, attr := range lbConf.Spec.LoadBalancerAttributes {
		attributes = append(attributes, elbv2model.LoadBalancerAttribute{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}
	return attributes
}
