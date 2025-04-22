package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

// FrontendNlbListenerConfig defines the configuration for an NLB listener
type FrontendNlbListenerConfig struct {
	Protocol                  elbv2model.Protocol
	Port                      int32
	TargetPort                int32
	HealthCheckConfig         elbv2model.TargetGroupHealthCheckConfig
	HealthCheckConfigExplicit map[string]bool
}

// FrontendNlbListenConfigWithIngress associates a listener config with its ingress resource
type FrontendNlbListenConfigWithIngress struct {
	ingKey                    types.NamespacedName
	FrontendNlbListenerConfig FrontendNlbListenerConfig
}

// buildFrontendNlbModel constructs the frontend NLB model for the ingress
// It creates the load balancer, listeners, and target groups based on ingress configurations
func (t *defaultModelBuildTask) buildFrontendNlbModel(ctx context.Context, alb *elbv2model.LoadBalancer, listenerPortConfigByIngress map[types.NamespacedName]map[int32]listenPortConfig) error {
	enableFrontendNlb, err := t.buildEnableFrontendNlbViaAnnotation(ctx)
	if err != nil {
		return err
	}

	// If the annotation is not present or explicitly set to false, do not build the NLB model
	if !enableFrontendNlb {
		return nil
	}

	scheme, err := t.buildFrontendNlbScheme(ctx, alb)
	if err != nil {
		return err
	}
	err = t.buildFrontendNlb(ctx, scheme, alb)
	if err != nil {
		return err
	}
	err = t.buildFrontendNlbListeners(ctx, listenerPortConfigByIngress)
	if err != nil {
		return err
	}
	return nil
}

func (t *defaultModelBuildTask) buildEnableFrontendNlbViaAnnotation(ctx context.Context) (bool, error) {
	var enableFrontendNlb *bool
	for _, member := range t.ingGroup.Members {
		rawEnableFrontendNlb := false
		exists, err := t.annotationParser.ParseBoolAnnotation(annotations.IngressSuffixEnableFrontendNlb, &rawEnableFrontendNlb, member.Ing.Annotations)
		if err != nil {
			return false, err
		}

		if exists {
			if enableFrontendNlb == nil {
				enableFrontendNlb = &rawEnableFrontendNlb
			} else if *enableFrontendNlb != rawEnableFrontendNlb {
				return false, errors.New("conflicting enable frontend NLB values")
			}

		}
	}

	if enableFrontendNlb == nil {
		return false, nil
	}

	return *enableFrontendNlb, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbScheme(ctx context.Context, alb *elbv2model.LoadBalancer) (elbv2model.LoadBalancerScheme, error) {
	scheme, explicitSchemeSpecified, err := t.buildFrontendNlbSchemeViaAnnotation(ctx, alb)
	if err != nil {
		return alb.Spec.Scheme, err
	}
	if explicitSchemeSpecified {
		return scheme, nil
	}

	return t.defaultScheme, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbSubnetMappings(ctx context.Context, scheme elbv2model.LoadBalancerScheme) ([]elbv2model.SubnetMapping, error) {
	var explicitSubnetNameOrIDsList [][]string
	for _, member := range t.ingGroup.Members {
		var rawSubnetNameOrIDs []string
		if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixFrontendNlbSubnets, &rawSubnetNameOrIDs, member.Ing.Annotations); !exists {
			continue
		}
		explicitSubnetNameOrIDsList = append(explicitSubnetNameOrIDsList, rawSubnetNameOrIDs)
	}

	if len(explicitSubnetNameOrIDsList) != 0 {
		chosenSubnetNameOrIDs := explicitSubnetNameOrIDsList[0]
		for _, subnetNameOrIDs := range explicitSubnetNameOrIDsList[1:] {
			if !cmp.Equal(chosenSubnetNameOrIDs, subnetNameOrIDs, equality.IgnoreStringSliceOrder()) {
				return nil, errors.Errorf("conflicting subnets: %v | %v", chosenSubnetNameOrIDs, subnetNameOrIDs)
			}
		}
		chosenSubnets, err := t.subnetsResolver.ResolveViaNameOrIDSlice(ctx, chosenSubnetNameOrIDs,
			networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
		if err != nil {
			return nil, err
		}

		return buildFrontendNlbSubnetMappingsWithSubnets(chosenSubnets), nil
	}

	return nil, nil

}

func buildFrontendNlbSubnetMappingsWithSubnets(subnets []ec2types.Subnet) []elbv2model.SubnetMapping {
	subnetMappings := make([]elbv2model.SubnetMapping, 0, len(subnets))
	for _, subnet := range subnets {
		subnetMappings = append(subnetMappings, elbv2model.SubnetMapping{
			SubnetID: awssdk.ToString(subnet.SubnetId),
		})
	}
	return subnetMappings
}

func (t *defaultModelBuildTask) buildFrontendNlb(ctx context.Context, scheme elbv2model.LoadBalancerScheme, alb *elbv2model.LoadBalancer) error {
	spec, err := t.buildFrontendNlbSpec(ctx, scheme, alb)
	if err != nil {
		return err
	}
	t.frontendNlb = elbv2model.NewLoadBalancer(t.stack, "FrontendNlb", spec)

	return nil
}

func (t *defaultModelBuildTask) buildFrontendNlbSpec(ctx context.Context, scheme elbv2model.LoadBalancerScheme,
	alb *elbv2model.LoadBalancer) (elbv2model.LoadBalancerSpec, error) {
	securityGroups, err := t.buildFrontendNlbSecurityGroups(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}

	// use alb security group if it is not explicitly specified
	if securityGroups == nil {
		securityGroups = alb.Spec.SecurityGroups
	}

	subnetMappings, err := t.buildFrontendNlbSubnetMappings(ctx, scheme)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}

	// use alb subnetMappings if it is not explicitly specified
	if subnetMappings == nil {
		subnetMappings = alb.Spec.SubnetMappings
	}

	name, err := t.buildFrontendNlbName(ctx, scheme, alb)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}

	spec := elbv2model.LoadBalancerSpec{
		Name:           name,
		Type:           elbv2model.LoadBalancerTypeNetwork,
		Scheme:         scheme,
		IPAddressType:  alb.Spec.IPAddressType,
		SecurityGroups: securityGroups,
		SubnetMappings: subnetMappings,
	}

	return spec, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbName(_ context.Context, scheme elbv2model.LoadBalancerScheme, alb *elbv2model.LoadBalancer) (string, error) {
	// build NLB name based upon ALB name
	// keeping as much of the original name as possible while ensuring the "-nlb" suffix is always present and the total length never exceeds 32 characters.
	if alb.Spec.Name != "" {
		baseName := alb.Spec.Name
		maxBaseLength := 28
		if len(baseName) > maxBaseLength {
			baseName = baseName[:maxBaseLength]
		}
		nlbName := baseName + "-nlb"

		return nlbName, nil
	}

	// Should not fall to this case, but keep it just in case
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.ingGroup.ID.String()))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	if t.ingGroup.ID.IsExplicit() {
		payload := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
		return fmt.Sprintf("k8s-%.16s-%.8s-nlb", payload, uuid), nil
	}

	sanitizedNamespace := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Namespace, "")
	sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.6s-nlb", sanitizedNamespace, sanitizedName, uuid), nil
}

func (t *defaultModelBuildTask) buildFrontendNlbSecurityGroups(ctx context.Context) ([]core.StringToken, error) {
	sgNameOrIDsViaAnnotation, err := t.buildNLBFrontendSGNameOrIDsFromAnnotation(ctx)
	if err != nil {
		return nil, err
	}

	var lbSGTokens []core.StringToken
	if len(sgNameOrIDsViaAnnotation) != 0 {
		frontendSGIDs, err := t.sgResolver.ResolveViaNameOrID(ctx, sgNameOrIDsViaAnnotation)

		if err != nil {
			return nil, err
		}
		for _, sgID := range frontendSGIDs {
			lbSGTokens = append(lbSGTokens, core.LiteralStringToken(sgID))
			return lbSGTokens, nil
		}
	}
	return nil, nil
}

func (t *defaultModelBuildTask) buildNLBFrontendSGNameOrIDsFromAnnotation(ctx context.Context) ([]string, error) {
	var explicitSGNameOrIDsList [][]string
	for _, member := range t.ingGroup.Members {
		var rawSGNameOrIDs []string
		if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixFrontendNlbSecurityGroups, &rawSGNameOrIDs, member.Ing.Annotations); !exists {
			continue
		}
		explicitSGNameOrIDsList = append(explicitSGNameOrIDsList, rawSGNameOrIDs)
	}
	if len(explicitSGNameOrIDsList) == 0 {
		return nil, nil
	}
	chosenSGNameOrIDs := explicitSGNameOrIDsList[0]
	for _, sgNameOrIDs := range explicitSGNameOrIDsList[1:] {
		if !cmp.Equal(chosenSGNameOrIDs, sgNameOrIDs) {
			return nil, errors.Errorf("conflicting securityGroups: %v | %v", chosenSGNameOrIDs, sgNameOrIDs)
		}
	}
	return chosenSGNameOrIDs, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbListeners(ctx context.Context, listenerPortConfigByIngress map[types.NamespacedName]map[int32]listenPortConfig) error {
	frontendNlbListenerConfigsByPort := make(map[int32][]FrontendNlbListenConfigWithIngress)

	// build frontend nlb config by port for ingress
	for _, member := range t.ingGroup.Members {
		ingKey := k8s.NamespacedName(member.Ing)
		frontendNlbListenerConfigByPortForIngress, err := t.buildFrontendNlbListenerConfigByPortForIngress(ctx, &member, listenerPortConfigByIngress)
		if err != nil {
			return errors.Wrapf(err, "failed to compute listenPort config for ingress: %s", ingKey.String())
		}
		for port, config := range frontendNlbListenerConfigByPortForIngress {
			configWithIngress := FrontendNlbListenConfigWithIngress{
				ingKey:                    ingKey,
				FrontendNlbListenerConfig: config,
			}
			frontendNlbListenerConfigsByPort[port] = append(
				frontendNlbListenerConfigsByPort[port],
				configWithIngress,
			)
		}
	}

	// merge frontend nlb listener configs
	frontendNlbListenerConfigByPort := make(map[int32]FrontendNlbListenerConfig)
	for port, cfgs := range frontendNlbListenerConfigsByPort {
		mergedCfg, err := t.mergeFrontendNlbListenPortConfigs(ctx, cfgs)
		if err != nil {
			return errors.Wrapf(err, "failed to merge NLB listenPort config for port: %v", port)
		}
		frontendNlbListenerConfigByPort[port] = mergedCfg
	}

	// build listener using the config
	for port, cfg := range frontendNlbListenerConfigByPort {
		_, err := t.buildFrontendNlbListener(ctx, port, cfg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *defaultModelBuildTask) buildFrontendNlbListenerConfigByPortForIngress(ctx context.Context, ing *ClassifiedIngress, listenerPortConfigByIngress map[types.NamespacedName]map[int32]listenPortConfig) (map[int32]FrontendNlbListenerConfig, error) {
	ingKey := k8s.NamespacedName(ing.Ing)

	frontendNlbListenerConfigByPort := make(map[int32]FrontendNlbListenerConfig)

	portMapping, err := t.parseFrontendNlbListenerPortMapping(ctx, ing.Ing.Annotations)
	if err != nil {
		return nil, err
	}

	// Check if frontend-nlb-listener-port-mapping exists
	if len(portMapping) > 0 {
		//if exists: only create NLB listeners for explicitly mapped ALB listener ports
		for nlbListenerPort, mappedAlbListenerPort := range portMapping {

			// check if the ALB listener port exists in the listener port set
			if _, exists := listenerPortConfigByIngress[ingKey][mappedAlbListenerPort]; !exists {
				t.logger.Info("Skipping NLB listener creation for unmapped ALB listener port", "mappedAlbListenerPort", mappedAlbListenerPort)
				continue
			}

			healthCheckConfig, isExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckConfig(ctx, ing.Ing.Annotations, "TCP")
			if err != nil {
				return nil, err
			}

			frontendNlbListenerConfigByPort[nlbListenerPort] = FrontendNlbListenerConfig{
				Protocol:                  elbv2model.ProtocolTCP,
				Port:                      nlbListenerPort,
				TargetPort:                mappedAlbListenerPort,
				HealthCheckConfig:         healthCheckConfig,
				HealthCheckConfigExplicit: isExplicit,
			}
		}

	} else {
		// if not: Map ALB listener ports directly to NLB listener ports
		for albListenerPort := range listenerPortConfigByIngress[ingKey] {

			healthCheckConfig, isExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckConfig(ctx, ing.Ing.Annotations, "TCP")
			if err != nil {
				return nil, err
			}

			// Add the listener configuration to the map
			frontendNlbListenerConfigByPort[albListenerPort] = FrontendNlbListenerConfig{
				Protocol:                  elbv2model.ProtocolTCP,
				Port:                      albListenerPort,
				TargetPort:                albListenerPort,
				HealthCheckConfig:         healthCheckConfig,
				HealthCheckConfigExplicit: isExplicit,
			}
		}
	}

	return frontendNlbListenerConfigByPort, nil
}

func (t *defaultModelBuildTask) parseFrontendNlbListenerPortMapping(ctx context.Context, ingAnnotation map[string]string) (map[int32]int32, error) {
	var rawPortMapping map[string]string
	_, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixFrontendNlbListenerPortMapping, &rawPortMapping, ingAnnotation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse frontend-nlb-listener-port-mapping for ingress %v", rawPortMapping)
	}

	portMappping := make(map[int32]int32)

	for rawNlbPort, rawAlbPort := range rawPortMapping {
		nlbPort, err := strconv.ParseInt(rawNlbPort, 10, 32)
		if err != nil {
			return nil, errors.Errorf("invalid NLB listener port: %s", rawNlbPort)
		}

		albPort, err := strconv.ParseInt(rawAlbPort, 10, 32)
		if err != nil {
			return nil, errors.Errorf("invalid ALB listener port: %s", rawAlbPort)
		}

		portMappping[int32(nlbPort)] = int32(albPort)
	}

	return portMappping, nil

}

func (t *defaultModelBuildTask) mergeFrontendNlbListenPortConfigs(ctx context.Context, configs []FrontendNlbListenConfigWithIngress) (FrontendNlbListenerConfig, error) {
	if len(configs) == 0 {
		return FrontendNlbListenerConfig{}, errors.New("no NLB listener port configurations provided")
	}

	// Initialize the final configuration
	finalConfig := FrontendNlbListenerConfig{}
	explicitFields := make(map[string]bool)

	// Port and Protocol are the same
	finalConfig.Port = configs[0].FrontendNlbListenerConfig.Port
	finalConfig.Protocol = configs[0].FrontendNlbListenerConfig.Protocol

	// Initialize the first Target port
	finalConfig.TargetPort = configs[0].FrontendNlbListenerConfig.TargetPort

	// Iterate over all configurations to build the final configuration
	for i, config := range configs {
		healthCheckConfig := config.FrontendNlbListenerConfig.HealthCheckConfig
		explicit := config.FrontendNlbListenerConfig.HealthCheckConfigExplicit

		// Merge intervalSeconds
		err := mergeHealthCheckField("IntervalSeconds", &finalConfig.HealthCheckConfig.IntervalSeconds, healthCheckConfig.IntervalSeconds, explicit, explicitFields, i)
		if err != nil {
			return FrontendNlbListenerConfig{}, err
		}

		// Merge timeoutSeconds
		err = mergeHealthCheckField("TimeoutSeconds", &finalConfig.HealthCheckConfig.TimeoutSeconds, healthCheckConfig.TimeoutSeconds, explicit, explicitFields, i)
		if err != nil {
			return FrontendNlbListenerConfig{}, err
		}

		// Merge healthyThresholdCount
		err = mergeHealthCheckField("HealthyThresholdCount", &finalConfig.HealthCheckConfig.HealthyThresholdCount, healthCheckConfig.HealthyThresholdCount, explicit, explicitFields, i)
		if err != nil {
			return FrontendNlbListenerConfig{}, err
		}

		// Merge unhealthyThresholdCount
		err = mergeHealthCheckField("UnhealthyThresholdCount", &finalConfig.HealthCheckConfig.UnhealthyThresholdCount, healthCheckConfig.UnhealthyThresholdCount, explicit, explicitFields, i)
		if err != nil {
			return FrontendNlbListenerConfig{}, err
		}

		// Merge protocol
		if explicit["Protocol"] {
			if explicitFields["Protocol"] {
				if finalConfig.HealthCheckConfig.Protocol != healthCheckConfig.Protocol {
					return FrontendNlbListenerConfig{}, errors.Errorf("conflicting Protocol, config %d: %s, previous: %s",
						i+1, healthCheckConfig.Protocol, finalConfig.HealthCheckConfig.Protocol)
				}
			} else {
				finalConfig.HealthCheckConfig.Protocol = healthCheckConfig.Protocol
				explicitFields["Protocol"] = true
			}
		} else if !explicitFields["Protocol"] {
			finalConfig.HealthCheckConfig.Protocol = healthCheckConfig.Protocol
		}

		// Merge path
		err = mergeHealthCheckField("Path", &finalConfig.HealthCheckConfig.Path, healthCheckConfig.Path, explicit, explicitFields, i)
		if err != nil {
			return FrontendNlbListenerConfig{}, err
		}

		// Merge matcher
		err = mergeHealthCheckField("Matcher", &finalConfig.HealthCheckConfig.Matcher, healthCheckConfig.Matcher, explicit, explicitFields, i)
		if err != nil {
			return FrontendNlbListenerConfig{}, err
		}

		// Merge port
		err = mergeHealthCheckField("Port", &finalConfig.HealthCheckConfig.Port, healthCheckConfig.Port, explicit, explicitFields, i)
		if err != nil {
			return FrontendNlbListenerConfig{}, err
		}

		// Validate NLB-to-ALB port mappings to ensure each NLB listener port maps to exactly one ALB port, preventing connection collisions
		if finalConfig.TargetPort != config.FrontendNlbListenerConfig.TargetPort {
			return FrontendNlbListenerConfig{}, errors.Errorf("conflicting Target Port, config %d: %v, previous: %v",
				i+1, config.FrontendNlbListenerConfig.TargetPort, finalConfig.TargetPort)
		} else {
			finalConfig.TargetPort = config.FrontendNlbListenerConfig.TargetPort
		}
	}

	return finalConfig, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbListener(ctx context.Context, port int32, config FrontendNlbListenerConfig) (*elbv2model.Listener, error) {
	lsSpec, err := t.buildFrontendNlbListenerSpec(ctx, port, config)
	if err != nil {
		return nil, err
	}
	frontendNlbListenerResID := buildFrontendNlbResourceID("ls", config.Protocol, &port)
	ls := elbv2model.NewListener(t.stack, frontendNlbListenerResID, lsSpec)
	return ls, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbListenerSpec(ctx context.Context, port int32, config FrontendNlbListenerConfig) (elbv2model.ListenerSpec, error) {
	listenerProtocol := elbv2model.Protocol(config.Protocol)

	targetGroup, err := t.buildFrontendNlbTargetGroup(ctx, port, config)
	if err != nil {
		return elbv2model.ListenerSpec{}, err
	}

	defaultActions := t.buildFrontendNlbListenerDefaultActions(ctx, targetGroup)

	t.frontendNlbTargetGroupDesiredState.AddTargetGroup(targetGroup.Spec.Name, targetGroup.TargetGroupARN(), t.loadBalancer.LoadBalancerARN(), *targetGroup.Spec.Port, config.TargetPort)

	return elbv2model.ListenerSpec{
		LoadBalancerARN: t.frontendNlb.LoadBalancerARN(),
		Port:            port,
		Protocol:        listenerProtocol,
		DefaultActions:  defaultActions,
	}, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbListenerDefaultActions(_ context.Context, targetGroup *elbv2model.TargetGroup) []elbv2model.Action {
	return []elbv2model.Action{
		{
			Type: elbv2model.ActionTypeForward,
			ForwardConfig: &elbv2model.ForwardActionConfig{
				TargetGroups: []elbv2model.TargetGroupTuple{
					{
						TargetGroupARN: targetGroup.TargetGroupARN(),
					},
				},
			},
		},
	}
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroup(ctx context.Context, port int32, config FrontendNlbListenerConfig) (*elbv2model.TargetGroup, error) {
	frontendNlbTgResID := buildFrontendNlbResourceID("tg", config.Protocol, &port)
	tgSpec, err := t.buildFrontendNlbTargetGroupSpec(ctx, config.Protocol, port, &config.HealthCheckConfig)
	if err != nil {
		return nil, err
	}
	targetGroup := elbv2model.NewTargetGroup(t.stack, frontendNlbTgResID, tgSpec)
	return targetGroup, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckConfig(ctx context.Context, svcAndIngAnnotations map[string]string, tgProtocol elbv2model.Protocol) (elbv2model.TargetGroupHealthCheckConfig, map[string]bool, error) {
	isExplicit := make(map[string]bool)

	healthCheckPort, portExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckPort(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, nil, err
	}
	isExplicit["Port"] = portExplicit

	healthCheckProtocol, protocolExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckProtocol(ctx, svcAndIngAnnotations, "HTTP")
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, nil, err
	}
	isExplicit["Protocol"] = protocolExplicit

	healthCheckPath, pathExplicit := t.buildFrontendNlbTargetGroupHealthCheckPath(ctx, svcAndIngAnnotations)
	isExplicit["Path"] = pathExplicit

	healthCheckMatcher, matcherExplicit := t.buildFrontendNlbTargetGroupHealthCheckMatcher(ctx, svcAndIngAnnotations, elbv2model.ProtocolVersionHTTP1)
	isExplicit["Matcher"] = matcherExplicit

	healthCheckIntervalSeconds, intervalExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckIntervalSeconds(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, nil, err
	}
	isExplicit["IntervalSeconds"] = intervalExplicit

	healthCheckTimeoutSeconds, timeoutExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckTimeoutSeconds(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, nil, err
	}
	isExplicit["TimeoutSeconds"] = timeoutExplicit

	healthCheckHealthyThresholdCount, healthyThresholdExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckHealthyThresholdCount(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, nil, err
	}
	isExplicit["HealthyThresholdCount"] = healthyThresholdExplicit

	healthCheckUnhealthyThresholdCount, unhealthyThresholdExplicit, err := t.buildFrontendNlbTargetGroupHealthCheckUnhealthyThresholdCount(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, nil, err
	}
	isExplicit["UnhealthyThresholdCount"] = unhealthyThresholdExplicit

	return elbv2model.TargetGroupHealthCheckConfig{
		Port:                    &healthCheckPort,
		Protocol:                healthCheckProtocol,
		Path:                    &healthCheckPath,
		Matcher:                 &healthCheckMatcher,
		IntervalSeconds:         awssdk.Int32(healthCheckIntervalSeconds),
		TimeoutSeconds:          awssdk.Int32(healthCheckTimeoutSeconds),
		HealthyThresholdCount:   awssdk.Int32(healthCheckHealthyThresholdCount),
		UnhealthyThresholdCount: awssdk.Int32(healthCheckUnhealthyThresholdCount),
	}, isExplicit, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckPort(_ context.Context, svcAndIngAnnotations map[string]string) (intstr.IntOrString, bool, error) {
	rawHealthCheckPort := ""
	exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixFrontendNlbHealthCheckPort, &rawHealthCheckPort, svcAndIngAnnotations)
	if !exists {
		return intstr.FromString(shared_constants.HealthCheckPortTrafficPort), false, nil
	}
	if rawHealthCheckPort == shared_constants.HealthCheckPortTrafficPort {
		return intstr.FromString(shared_constants.HealthCheckPortTrafficPort), true, nil
	}
	healthCheckPort := intstr.Parse(rawHealthCheckPort)
	if healthCheckPort.Type == intstr.Int {
		return healthCheckPort, true, nil
	}

	return intstr.IntOrString{}, true, errors.New("failed to resolve healthCheckPort")
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckProtocol(_ context.Context, svcAndIngAnnotations map[string]string, tgProtocol elbv2model.Protocol) (elbv2model.Protocol, bool, error) {
	rawHealthCheckProtocol := string(tgProtocol)
	exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixFrontendNlbHealthCheckProtocol, &rawHealthCheckProtocol, svcAndIngAnnotations)
	switch rawHealthCheckProtocol {
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, exists, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, exists, nil
	default:
		return "", exists, errors.Errorf("healthCheckProtocol must be within [%v, %v]", elbv2model.ProtocolHTTP, elbv2model.ProtocolHTTPS)
	}
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckPath(_ context.Context, svcAndIngAnnotations map[string]string) (string, bool) {
	rawHealthCheckPath := t.defaultHealthCheckPathHTTP
	exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixFrontendNlbHealthCheckPath, &rawHealthCheckPath, svcAndIngAnnotations)
	return rawHealthCheckPath, exists
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckMatcher(_ context.Context, svcAndIngAnnotations map[string]string, tgProtocolVersion elbv2model.ProtocolVersion) (elbv2model.HealthCheckMatcher, bool) {
	rawHealthCheckMatcherHTTPCode := t.defaultHealthCheckMatcherHTTPCode
	exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixFrontendNlbHealthCheckSuccessCodes, &rawHealthCheckMatcherHTTPCode, svcAndIngAnnotations)
	return elbv2model.HealthCheckMatcher{
		HTTPCode: &rawHealthCheckMatcherHTTPCode,
	}, exists
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckIntervalSeconds(_ context.Context, svcAndIngAnnotations map[string]string) (int32, bool, error) {
	rawHealthCheckIntervalSeconds := t.defaultHealthCheckIntervalSeconds
	exists, err := t.annotationParser.ParseInt32Annotation(annotations.IngressSuffixFrontendNlbHealthCheckIntervalSeconds, &rawHealthCheckIntervalSeconds, svcAndIngAnnotations)
	if err != nil {
		return 0, false, err
	}
	return rawHealthCheckIntervalSeconds, exists, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckTimeoutSeconds(_ context.Context, svcAndIngAnnotations map[string]string) (int32, bool, error) {
	rawHealthCheckTimeoutSeconds := t.defaultHealthCheckTimeoutSeconds
	exists, err := t.annotationParser.ParseInt32Annotation(annotations.IngressSuffixFrontendNlbHealthCheckTimeoutSeconds, &rawHealthCheckTimeoutSeconds, svcAndIngAnnotations)
	if err != nil {
		return 0, false, err
	}
	return rawHealthCheckTimeoutSeconds, exists, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckHealthyThresholdCount(_ context.Context, svcAndIngAnnotations map[string]string) (int32, bool, error) {
	rawHealthCheckHealthyThresholdCount := t.defaultHealthCheckHealthyThresholdCount
	exists, err := t.annotationParser.ParseInt32Annotation(annotations.IngressSuffixFrontendNlbHealthCheckHealthyThresholdCount,
		&rawHealthCheckHealthyThresholdCount, svcAndIngAnnotations)
	if err != nil {
		return 0, false, err
	}
	return rawHealthCheckHealthyThresholdCount, exists, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupHealthCheckUnhealthyThresholdCount(_ context.Context, svcAndIngAnnotations map[string]string) (int32, bool, error) {
	rawHealthCheckUnhealthyThresholdCount := t.defaultHealthCheckUnhealthyThresholdCount
	exists, err := t.annotationParser.ParseInt32Annotation(annotations.IngressSuffixFrontendNlHealthCheckbUnhealthyThresholdCount,
		&rawHealthCheckUnhealthyThresholdCount, svcAndIngAnnotations)
	if err != nil {
		return 0, false, err
	}
	return rawHealthCheckUnhealthyThresholdCount, exists, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupSpec(ctx context.Context, tgProtocol elbv2model.Protocol,
	port int32, healthCheckConfig *elbv2model.TargetGroupHealthCheckConfig) (elbv2model.TargetGroupSpec, error) {

	tgName := t.buildFrontendNlbTargetGroupName(ctx, port, elbv2model.TargetTypeALB, tgProtocol, healthCheckConfig)

	return elbv2model.TargetGroupSpec{
		Name:              tgName,
		TargetType:        elbv2model.TargetTypeALB,
		Port:              awssdk.Int32(port),
		Protocol:          tgProtocol,
		IPAddressType:     elbv2model.TargetGroupIPAddressType(t.loadBalancer.Spec.IPAddressType),
		HealthCheckConfig: healthCheckConfig,
	}, nil
}

func (t *defaultModelBuildTask) buildFrontendNlbTargetGroupName(_ context.Context, tgPort int32,
	targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol, hc *elbv2model.TargetGroupHealthCheckConfig) string {
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.ingGroup.ID.String()))
	_, _ = uuidHash.Write([]byte(strconv.Itoa(int(tgPort))))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(tgProtocol))
	_, _ = uuidHash.Write([]byte(hc.Port.String()))
	_, _ = uuidHash.Write([]byte(hc.Protocol))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	if t.ingGroup.ID.IsExplicit() {
		sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
		return fmt.Sprintf("k8s-%.17s-%.10s", sanitizedName, uuid)
	}

	sanitizedNamespace := invalidTargetGroupNamePattern.ReplaceAllString(t.ingGroup.ID.Namespace, "")
	sanitizedName := invalidTargetGroupNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildFrontendNlbSchemeViaAnnotation(ctx context.Context, alb *elbv2model.LoadBalancer) (elbv2model.LoadBalancerScheme, bool, error) {
	explicitSchemes := sets.Set[string]{}
	for _, member := range t.ingGroup.Members {
		rawSchema := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixFrontendNlbScheme, &rawSchema, member.Ing.Annotations); !exists {
			continue
		}
		explicitSchemes.Insert(rawSchema)
	}
	if len(explicitSchemes) == 0 {
		return alb.Spec.Scheme, false, nil
	}
	if len(explicitSchemes) > 1 {
		return "", true, errors.Errorf("conflicting scheme: %v", explicitSchemes)
	}
	rawScheme, _ := explicitSchemes.PopAny()
	switch rawScheme {
	case string(elbv2model.LoadBalancerSchemeInternetFacing):
		return elbv2model.LoadBalancerSchemeInternetFacing, true, nil
	case string(elbv2model.LoadBalancerSchemeInternal):
		return elbv2model.LoadBalancerSchemeInternal, true, nil
	default:
		return "", false, errors.Errorf("unknown scheme: %v", rawScheme)
	}
}

func buildFrontendNlbResourceID(resourceType string, protocol elbv2model.Protocol, port *int32) string {
	if port != nil && protocol != "" {
		return fmt.Sprintf("FrontendNlb-%s-%v-%v", resourceType, protocol, *port)
	}
	return fmt.Sprintf("FrontendNlb")
}

func mergeHealthCheckField[T comparable](fieldName string, finalValue **T, currentValue *T, explicit map[string]bool, explicitFields map[string]bool, configIndex int) error {
	if explicit[fieldName] {
		if explicitFields[fieldName] {
			fmt.Printf("*finalValue (%T): %v\n", **finalValue, **finalValue)
			fmt.Printf("newValue (%T): %v\n", *currentValue, *currentValue)
			if **finalValue != *currentValue {
				return errors.Errorf("conflicting %s, config %d: %v, previous: %v", fieldName, configIndex+1, *currentValue, **finalValue)
			}
		} else {
			*finalValue = currentValue
			explicitFields[fieldName] = true
		}
	} else if !explicitFields[fieldName] {
		*finalValue = currentValue
	}
	return nil
}
