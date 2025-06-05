package gateway

import (
	"github.com/go-logr/logr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
)

const (
	kindNsRank = 2
	kindRank   = 1
)

type TargetGroupConfigConstructor interface {
	ConstructTargetGroupConfigForRoute(tgConfig *elbv2gw.TargetGroupConfiguration, name, namespace, kind string) *elbv2gw.TargetGroupProps
}

type targetGroupConfigConstructorImpl struct {
	logger logr.Logger
}

func (t *targetGroupConfigConstructorImpl) ConstructTargetGroupConfigForRoute(tgConfig *elbv2gw.TargetGroupConfiguration, routeName, routeNamespace, routeKind string) *elbv2gw.TargetGroupProps {
	if tgConfig == nil {
		return nil
	}
	cfgCopy := tgConfig.DeepCopy()
	return t.mergeWithLongestMatch(&cfgCopy.Spec.DefaultConfiguration, cfgCopy.Spec.RouteConfigurations, routeName, routeNamespace, routeKind)
}

func (t *targetGroupConfigConstructorImpl) mergeWithLongestMatch(initialCfg *elbv2gw.TargetGroupProps, routeConfigurations []elbv2gw.RouteConfiguration, routeName, routeNamespace, routeKind string) *elbv2gw.TargetGroupProps {
	var matched *elbv2gw.TargetGroupProps
	longestMatch := 0

	for _, routeConfig := range routeConfigurations {
		kindEquals := routeConfig.RouteIdentifier.RouteKind == routeKind
		nsEquals := routeConfig.RouteIdentifier.RouteNamespace == routeNamespace
		nameEquals := routeConfig.RouteIdentifier.RouteName == routeName
		if kindEquals && nsEquals && nameEquals {
			// Complete match kind:ns:name
			matched = &routeConfig.TargetGroupProps
			break
		}

		idHasName := routeConfig.RouteIdentifier.RouteName != ""
		idHasNamespace := routeConfig.RouteIdentifier.RouteNamespace != ""

		if idHasName {
			continue
		}

		if (kindEquals && nsEquals) && kindNsRank > longestMatch {
			// Partial match kind:ns
			matched = &routeConfig.TargetGroupProps
			longestMatch = kindNsRank
		} else if (kindEquals && !idHasNamespace) && kindRank > longestMatch {
			// Partial match kind
			matched = &routeConfig.TargetGroupProps
			longestMatch = kindRank
		}
	}
	return t.merge(matched, initialCfg)
}

func (t *targetGroupConfigConstructorImpl) merge(highPriority *elbv2gw.TargetGroupProps, defaultProps *elbv2gw.TargetGroupProps) *elbv2gw.TargetGroupProps {
	if highPriority == nil {
		return defaultProps
	}
	var result elbv2gw.TargetGroupProps

	t.performTakeOneMerges(&result, highPriority, defaultProps)
	result.Tags = mergeTags(highPriority.Tags, defaultProps.Tags)
	result.TargetGroupAttributes = mergeAttributes(highPriority.TargetGroupAttributes, defaultProps.TargetGroupAttributes, targetGroupAttributeKeyFn, targetGroupAttributeValueFn, targetGroupAttributeConstructor)
	return &result
}

func (t *targetGroupConfigConstructorImpl) performTakeOneMerges(merged, highPriority, defaultProps *elbv2gw.TargetGroupProps) {
	if highPriority.TargetGroupName != nil {
		merged.TargetGroupName = highPriority.TargetGroupName
	} else {
		merged.TargetGroupName = defaultProps.TargetGroupName
	}

	if highPriority.IPAddressType != nil {
		merged.IPAddressType = highPriority.IPAddressType
	} else {
		merged.IPAddressType = defaultProps.IPAddressType
	}

	if highPriority.HealthCheckConfig != nil {
		merged.HealthCheckConfig = highPriority.HealthCheckConfig
	} else {
		merged.HealthCheckConfig = defaultProps.HealthCheckConfig
	}

	if highPriority.NodeSelector != nil {
		merged.NodeSelector = highPriority.NodeSelector
	} else {
		merged.NodeSelector = defaultProps.NodeSelector
	}

	if highPriority.TargetType != nil {
		merged.TargetType = highPriority.TargetType
	} else {
		merged.TargetType = defaultProps.TargetType
	}

	if highPriority.Protocol != nil {
		merged.Protocol = highPriority.Protocol
	} else {
		merged.Protocol = defaultProps.Protocol
	}

	if highPriority.ProtocolVersion != nil {
		merged.ProtocolVersion = highPriority.ProtocolVersion
	} else {
		merged.ProtocolVersion = defaultProps.ProtocolVersion
	}

	if highPriority.EnableMultiCluster != nil {
		merged.EnableMultiCluster = highPriority.EnableMultiCluster
	} else {
		merged.EnableMultiCluster = defaultProps.EnableMultiCluster
	}

	if highPriority.EnableMultiCluster != nil {
		merged.EnableMultiCluster = highPriority.EnableMultiCluster
	} else {
		merged.EnableMultiCluster = defaultProps.EnableMultiCluster
	}
}

func NewTargetGroupConfigConstructor(logger logr.Logger) TargetGroupConfigConstructor {
	return &targetGroupConfigConstructorImpl{
		logger: logger,
	}
}
