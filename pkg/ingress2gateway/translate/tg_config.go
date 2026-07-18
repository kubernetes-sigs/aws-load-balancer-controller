package translate

import (
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway/utils"
	sharedconstants "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/shared_constants"
)

// buildTargetGroupConfig builds a TargetGroupConfiguration for a given service from annotations.
// Returns nil if no TG-level annotations are present.
func buildTargetGroupConfig(svcRef serviceRef, annos map[string]string, migrationTag string) *gatewayv1beta1.TargetGroupConfiguration {
	props := buildTargetGroupProps(annos, svcRef.name, svcRef.port)

	if reflect.DeepEqual(props, gatewayv1beta1.TargetGroupProps{}) {
		return nil
	}

	// Add migration tag only when we have real config
	if migrationTag != "" {
		if props.Tags == nil {
			tags := make(map[string]string)
			props.Tags = &tags
		}
		(*props.Tags)[utils.MigrationTagKey] = migrationTag
	}

	return &gatewayv1beta1.TargetGroupConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: utils.LBConfigAPIVersion,
			Kind:       utils.TGConfigKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GetTGConfigName(svcRef.namespace, svcRef.name),
			Namespace: svcRef.namespace,
		},
		Spec: gatewayv1beta1.TargetGroupConfigurationSpec{
			TargetReference: &gatewayv1beta1.Reference{
				Name: svcRef.name,
			},
			DefaultConfiguration: props,
		},
	}
}

// buildTargetGroupConfigFromEntries builds a TargetGroupConfiguration from one or more
// ingress entries referencing the same service. A single entry uses DefaultConfiguration.
// Multiple entries always emit RouteConfigurations keyed by the generated HTTPRoute name,
// so the controller's longest-match merge assigns each HTTPRoute its own TG settings.
func buildTargetGroupConfigFromEntries(entries []tgcEntry) *gatewayv1beta1.TargetGroupConfiguration {
	if len(entries) == 0 {
		return nil
	}

	// All entries share the same svcRef (grouped by namespace/name:port).
	svcRef := entries[0].svcRef

	// Single entry — use DefaultConfiguration (simple path).
	if len(entries) == 1 {
		tgc := buildTargetGroupConfig(svcRef, entries[0].annotations, entries[0].migrationTag)
		if tgc != nil {
			for _, icp := range entries[0].icps {
				applyIngressClassParamsToTGProps(&tgc.Spec.DefaultConfiguration, icp)
			}
		}
		return tgc
	}

	// Multiple entries — emit a RouteConfiguration per ingress.
	tgc := &gatewayv1beta1.TargetGroupConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: utils.LBConfigAPIVersion,
			Kind:       utils.TGConfigKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GetTGConfigName(svcRef.namespace, svcRef.name),
			Namespace: svcRef.namespace,
		},
		Spec: gatewayv1beta1.TargetGroupConfigurationSpec{
			TargetReference: &gatewayv1beta1.Reference{
				Name: svcRef.name,
			},
		},
	}

	for _, entry := range entries {
		props := buildTargetGroupProps(entry.annotations, svcRef.name, svcRef.port)

		// Apply ICP overrides from this entry's group.
		for _, icp := range entry.icps {
			applyIngressClassParamsToTGProps(&props, icp)
		}

		// Add migration tag.
		if entry.migrationTag != "" {
			if props.Tags == nil {
				tags := make(map[string]string)
				props.Tags = &tags
			}
			(*props.Tags)[utils.MigrationTagKey] = entry.migrationTag
		}

		tgc.Spec.RouteConfigurations = append(tgc.Spec.RouteConfigurations, gatewayv1beta1.RouteConfiguration{
			RouteIdentifier: gatewayv1beta1.RouteIdentifier{
				RouteKind:      sharedconstants.HTTPRouteKind,
				RouteNamespace: svcRef.namespace,
				RouteName:      entry.routeName,
			},
			TargetGroupProps: props,
		})
	}

	return tgc
}

// buildTargetGroupProps builds TargetGroupProps from annotations.
func buildTargetGroupProps(annos map[string]string, serviceName string, servicePort int32) gatewayv1beta1.TargetGroupProps {
	props := gatewayv1beta1.TargetGroupProps{}

	if v := getString(annos, annotations.IngressSuffixTargetType); v != "" {
		tt := gatewayv1beta1.TargetType(v)
		props.TargetType = &tt
	}

	if v := getString(annos, annotations.IngressSuffixBackendProtocol); v != "" {
		p := gatewayv1beta1.Protocol(v)
		props.Protocol = &p
	}

	if v := getString(annos, annotations.IngressSuffixBackendProtocolVersion); v != "" {
		pv := gatewayv1beta1.ProtocolVersion(v)
		props.ProtocolVersion = &pv
	}

	if attrs := getStringMap(annos, annotations.IngressSuffixTargetGroupAttributes); len(attrs) > 0 {
		for k, v := range attrs {
			props.TargetGroupAttributes = append(props.TargetGroupAttributes, gatewayv1beta1.TargetGroupAttribute{
				Key:   k,
				Value: v,
			})
		}
	}

	if labels := getStringMap(annos, annotations.IngressSuffixTargetNodeLabels); len(labels) > 0 {
		props.NodeSelector = &metav1.LabelSelector{
			MatchLabels: labels,
		}
	}

	if v := getBool(annos, annotations.IngressLBSuffixMultiClusterTargetGroup); v != nil {
		props.EnableMultiCluster = v
	}

	if tags := getStringMap(annos, annotations.IngressSuffixTags); len(tags) > 0 {
		props.Tags = &tags
	}

	// target-control-port.${serviceName}.${servicePort}
	if serviceName != "" && servicePort > 0 {
		targetControlPortSuffix := fmt.Sprintf("%s.%s.%d", annotations.IngressSuffixTargetControlPort, serviceName, servicePort)
		if v := getInt32(annos, targetControlPortSuffix); v != nil {
			props.TargetControlPort = v
		}
	}

	hc := buildHealthCheckConfig(annos)
	if hc != nil {
		props.HealthCheckConfig = hc
	}

	return props
}

// buildHealthCheckConfig builds HealthCheckConfiguration from healthcheck-* annotations.
func buildHealthCheckConfig(annos map[string]string) *gatewayv1beta1.HealthCheckConfiguration {
	hc := &gatewayv1beta1.HealthCheckConfiguration{}
	hasAny := false

	if v := getString(annos, annotations.IngressSuffixHealthCheckPort); v != "" {
		hc.HealthCheckPort = &v
		hasAny = true
	}

	if v := getString(annos, annotations.IngressSuffixHealthCheckProtocol); v != "" {
		p := gatewayv1beta1.TargetGroupHealthCheckProtocol(v)
		hc.HealthCheckProtocol = &p
		hasAny = true
	}

	if v := getString(annos, annotations.IngressSuffixHealthCheckPath); v != "" {
		hc.HealthCheckPath = &v
		hasAny = true
	}

	if v := getInt32(annos, annotations.IngressSuffixHealthCheckIntervalSeconds); v != nil {
		hc.HealthCheckInterval = v
		hasAny = true
	}

	if v := getInt32(annos, annotations.IngressSuffixHealthCheckTimeoutSeconds); v != nil {
		hc.HealthCheckTimeout = v
		hasAny = true
	}

	if v := getInt32(annos, annotations.IngressSuffixHealthyThresholdCount); v != nil {
		hc.HealthyThresholdCount = v
		hasAny = true
	}

	if v := getInt32(annos, annotations.IngressSuffixUnhealthyThresholdCount); v != nil {
		hc.UnhealthyThresholdCount = v
		hasAny = true
	}

	if v := getString(annos, annotations.IngressSuffixSuccessCodes); v != "" {
		hc.Matcher = &gatewayv1beta1.HealthCheckMatcher{
			HTTPCode: &v,
		}
		hasAny = true
	}

	if !hasAny {
		return nil
	}
	return hc
}
