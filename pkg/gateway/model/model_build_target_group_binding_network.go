package model

import (
	"context"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

type targetGroupBindingNetworkBuilder interface {
	buildTargetGroupBindingNetworking(targetGroupSpec elbv2model.TargetGroupSpec, targetPort intstr.IntOrString) (*elbv2modelk8s.TargetGroupBindingNetworking, error)
}

type targetGroupBindingNetworkBuilderImpl struct {
	log                      logr.Logger
	disableRestrictedSGRules bool
	vpcID                    string
	sgOutput                 securityGroupOutput
	loadBalancerSubnets      []ec2types.Subnet
	lbScheme                 elbv2model.LoadBalancerScheme
	lbSourceRanges           *[]string
	vpcInfoProvider          networking.VPCInfoProvider
}

func newTargetGroupBindingNetworkBuilder(disableRestrictedSGRules bool, vpcID string, lbScheme elbv2model.LoadBalancerScheme, lbSourceRanges *[]string, sgOutput securityGroupOutput, loadBalancerSubnets []ec2types.Subnet, vpcInfoProvider networking.VPCInfoProvider) targetGroupBindingNetworkBuilder {
	return &targetGroupBindingNetworkBuilderImpl{
		disableRestrictedSGRules: disableRestrictedSGRules,
		lbScheme:                 lbScheme,
		lbSourceRanges:           lbSourceRanges,
		vpcID:                    vpcID,
		sgOutput:                 sgOutput,
		loadBalancerSubnets:      loadBalancerSubnets,
		vpcInfoProvider:          vpcInfoProvider,
	}
}

func (builder *targetGroupBindingNetworkBuilderImpl) buildTargetGroupBindingNetworking(targetGroupSpec elbv2model.TargetGroupSpec, targetPort intstr.IntOrString) (*elbv2modelk8s.TargetGroupBindingNetworking, error) {
	if len(builder.sgOutput.securityGroupTokens) == 0 {
		return builder.nlbNoSecurityGroups(targetPort, targetGroupSpec)
	}
	return builder.standardBuilder(targetPort, *targetGroupSpec.HealthCheckConfig.Port, targetGroupSpec.Protocol, targetGroupSpec.TargetControlPort), nil
}

func (builder *targetGroupBindingNetworkBuilderImpl) standardBuilder(targetPort intstr.IntOrString, healthCheckPort intstr.IntOrString, tgProtocol elbv2model.Protocol, targetControlPort *int32) *elbv2modelk8s.TargetGroupBindingNetworking {
	if builder.sgOutput.backendSecurityGroupToken == nil {
		return nil
	}
	protocolTCP := elbv2api.NetworkingProtocolTCP
	protocolUDP := elbv2api.NetworkingProtocolUDP

	udpSupported := tgProtocol == elbv2model.ProtocolUDP || tgProtocol == elbv2model.ProtocolTCP_UDP

	if builder.disableRestrictedSGRules {
		ports := []elbv2api.NetworkingPort{
			{
				Protocol: &protocolTCP,
				Port:     nil,
			},
		}

		if udpSupported {
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolUDP,
				Port:     nil,
			})
		}

		return &elbv2modelk8s.TargetGroupBindingNetworking{
			Ingress: []elbv2modelk8s.NetworkingIngressRule{
				{
					From: []elbv2modelk8s.NetworkingPeer{
						{
							SecurityGroup: &elbv2modelk8s.SecurityGroup{
								GroupID: builder.sgOutput.backendSecurityGroupToken,
							},
						},
					},
					Ports: ports,
				},
			},
		}
	}

	var networkingPorts []elbv2api.NetworkingPort

	protocolToUse := &protocolTCP
	if udpSupported {
		protocolToUse = &protocolUDP
	}

	networkingPorts = append(networkingPorts, elbv2api.NetworkingPort{
		Protocol: protocolToUse,
		Port:     &targetPort,
	})

	if udpSupported || (healthCheckPort.Type == intstr.Int && healthCheckPort.IntValue() != targetPort.IntValue()) {
		var hcPortToUse intstr.IntOrString
		if healthCheckPort.Type == intstr.String {
			hcPortToUse = targetPort
		} else {
			hcPortToUse = healthCheckPort
		}

		networkingPorts = append(networkingPorts, elbv2api.NetworkingPort{
			Protocol: &protocolTCP,
			Port:     &hcPortToUse,
		})
	}

	if targetControlPort != nil {
		controlPort := intstr.FromInt32(*targetControlPort)
		networkingPorts = append(networkingPorts, elbv2api.NetworkingPort{
			Protocol: &protocolTCP,
			Port:     &controlPort,
		})
	}

	var networkingRules []elbv2modelk8s.NetworkingIngressRule
	for _, port := range networkingPorts {
		networkingRules = append(networkingRules, elbv2modelk8s.NetworkingIngressRule{
			From: []elbv2modelk8s.NetworkingPeer{
				{
					SecurityGroup: &elbv2modelk8s.SecurityGroup{
						GroupID: builder.sgOutput.backendSecurityGroupToken,
					},
				},
			},
			Ports: []elbv2api.NetworkingPort{port},
		})
	}
	return &elbv2modelk8s.TargetGroupBindingNetworking{
		Ingress: networkingRules,
	}
}

func (builder *targetGroupBindingNetworkBuilderImpl) nlbNoSecurityGroups(targetPort intstr.IntOrString, tgSpec elbv2model.TargetGroupSpec) (*elbv2modelk8s.TargetGroupBindingNetworking, error) {
	healthCheckProtocol := elbv2api.NetworkingProtocolTCP
	healthCheckPort := *tgSpec.HealthCheckConfig.Port
	var err error

	loadBalancerSubnetCIDRs := builder.parseSubnetCIDRBlocks(tgSpec.IPAddressType, builder.loadBalancerSubnets)
	trafficSource := loadBalancerSubnetCIDRs
	defaultRangeUsed := false
	var trafficPorts []elbv2api.NetworkingPort

	isPreserveClientIP := builder.getPreserveClientIP(tgSpec)

	if isPreserveClientIP {
		if builder.lbSourceRanges != nil {
			trafficSource = *builder.lbSourceRanges
		} else {
			trafficSource = []string{}
		}
		if len(trafficSource) == 0 {
			trafficSource, err = builder.getDefaultIPSourceRanges(isPreserveClientIP, tgSpec)
			if err != nil {
				return nil, err
			}
			defaultRangeUsed = true
		}
	}

	if tgSpec.Protocol == elbv2model.ProtocolTCP_UDP {
		tcpProtocol := elbv2api.NetworkingProtocolTCP
		udpProtocol := elbv2api.NetworkingProtocolUDP
		trafficPorts = []elbv2api.NetworkingPort{
			{
				Port:     &targetPort,
				Protocol: &tcpProtocol,
			},
			{
				Port:     &targetPort,
				Protocol: &udpProtocol,
			},
		}
	} else {
		networkingProtocol := elbv2api.NetworkingProtocolTCP
		if tgSpec.Protocol == elbv2model.ProtocolUDP {
			networkingProtocol = elbv2api.NetworkingProtocolUDP
		}

		trafficPorts = []elbv2api.NetworkingPort{
			{
				Port:     &targetPort,
				Protocol: &networkingProtocol,
			},
		}
	}

	tgbNetworking := &elbv2modelk8s.TargetGroupBindingNetworking{
		Ingress: []elbv2modelk8s.NetworkingIngressRule{
			{
				From:  builder.buildPeersFromSourceRangeCIDRs(trafficSource),
				Ports: trafficPorts,
			},
		},
	}

	if healthCheckSourceCIDRs := builder.buildHealthCheckSourceCIDRs(isPreserveClientIP, trafficSource, loadBalancerSubnetCIDRs, targetPort, healthCheckPort,
		tgSpec.Protocol, defaultRangeUsed); len(healthCheckSourceCIDRs) > 0 {
		networkingHealthCheckPort := healthCheckPort
		if healthCheckPort.String() == shared_constants.HealthCheckPortTrafficPort {
			networkingHealthCheckPort = targetPort
		}
		tgbNetworking.Ingress = append(tgbNetworking.Ingress, elbv2modelk8s.NetworkingIngressRule{
			From: builder.buildPeersFromSourceRangeCIDRs(healthCheckSourceCIDRs),
			Ports: []elbv2api.NetworkingPort{
				{
					Port:     &networkingHealthCheckPort,
					Protocol: &healthCheckProtocol,
				},
			},
		})
	}
	return tgbNetworking, nil
}

func (builder *targetGroupBindingNetworkBuilderImpl) getPreserveClientIP(tgSpec elbv2model.TargetGroupSpec) bool {
	/*
		https://docs.aws.amazon.com/elasticloadbalancing/latest/network/edit-target-group-attributes.html#client-ip-preservation
		By default, client IP preservation is enabled (and can't be disabled) for instance and IP type target groups with UDP and TCP_UDP protocols.
		However, you can enable or disable client IP preservation for TCP and TLS target groups using the preserve_client_ip.enabled target group attribute.
	*/

	if tgSpec.Protocol == elbv2model.ProtocolUDP || tgSpec.Protocol == elbv2model.ProtocolTCP_UDP {
		return true
	}

	for _, attr := range tgSpec.TargetGroupAttributes {
		if attr.Key == shared_constants.TGAttributePreserveClientIPEnabled {
			v, err := strconv.ParseBool(attr.Value)
			if err != nil {
				return false
			}
			return v
		}
	}
	return tgSpec.TargetType == elbv2model.TargetTypeInstance
}

func (builder *targetGroupBindingNetworkBuilderImpl) getDefaultIPSourceRanges(preserveClientIP bool, tgSpec elbv2model.TargetGroupSpec) ([]string, error) {
	defaultSourceRanges := []string{"0.0.0.0/0"}
	if tgSpec.IPAddressType == elbv2model.TargetGroupIPAddressTypeIPv6 {
		defaultSourceRanges = []string{"::/0"}
	}
	if (preserveClientIP) && builder.lbScheme == elbv2model.LoadBalancerSchemeInternal {
		vpcInfo, err := builder.vpcInfoProvider.FetchVPCInfo(context.Background(), builder.vpcID, networking.FetchVPCInfoWithoutCache())
		if err != nil {
			return nil, err
		}
		if tgSpec.IPAddressType == elbv2model.TargetGroupIPAddressTypeIPv4 {
			defaultSourceRanges = vpcInfo.AssociatedIPv4CIDRs()
		} else {
			defaultSourceRanges = vpcInfo.AssociatedIPv6CIDRs()
		}
	}
	return defaultSourceRanges, nil
}

func (builder *targetGroupBindingNetworkBuilderImpl) parseSubnetCIDRBlocks(ipType elbv2model.TargetGroupIPAddressType, subnets []ec2types.Subnet) []string {
	var subnetCIDRs []string
	for _, subnet := range subnets {
		if ipType == elbv2model.TargetGroupIPAddressTypeIPv4 {
			subnetCIDRs = append(subnetCIDRs, awssdk.ToString(subnet.CidrBlock))
		} else {
			for _, ipv6CIDRBlockAssoc := range subnet.Ipv6CidrBlockAssociationSet {
				subnetCIDRs = append(subnetCIDRs, awssdk.ToString(ipv6CIDRBlockAssoc.Ipv6CidrBlock))
			}
		}
	}
	return subnetCIDRs
}
func (builder *targetGroupBindingNetworkBuilderImpl) buildPeersFromSourceRangeCIDRs(sourceRanges []string) []elbv2modelk8s.NetworkingPeer {
	var peers []elbv2modelk8s.NetworkingPeer
	for _, cidr := range sourceRanges {
		peers = append(peers, elbv2modelk8s.NetworkingPeer{
			IPBlock: &elbv2api.IPBlock{
				CIDR: cidr,
			},
		})
	}
	return peers
}

func (builder *targetGroupBindingNetworkBuilderImpl) buildHealthCheckSourceCIDRs(preserveClientIP bool, trafficSource, subnetCIDRs []string, tgPort, hcPort intstr.IntOrString,
	tgProtocol elbv2model.Protocol, defaultRangeUsed bool) []string {
	if tgProtocol != elbv2model.ProtocolUDP &&
		(hcPort.String() == shared_constants.HealthCheckPortTrafficPort || hcPort.IntValue() == tgPort.IntValue()) {
		if !preserveClientIP {
			return nil
		}
		if defaultRangeUsed {
			return nil
		}
		for _, src := range trafficSource {
			if src == "0.0.0.0/0" || src == "::/0" {
				return nil
			}
		}
	}
	return subnetCIDRs
}
