package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/utils/strings/slices"
	"net"
	"strings"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func (t *defaultModelBuildTask) buildListener(ctx context.Context, lbARN core.StringToken, port int64, config listenPortConfig, ingList []ClassifiedIngress) (*elbv2model.Listener, error) {
	lsSpec, err := t.buildListenerSpec(ctx, lbARN, port, config, ingList)
	if err != nil {
		return nil, err
	}
	lsResID := fmt.Sprintf("%v", port)
	ls := elbv2model.NewListener(t.stack, lsResID, lsSpec)
	return ls, nil
}

func (t *defaultModelBuildTask) buildListenerSpec(ctx context.Context, lbARN core.StringToken, port int64, config listenPortConfig, ingList []ClassifiedIngress) (elbv2model.ListenerSpec, error) {
	defaultActions, err := t.buildListenerDefaultActions(ctx, config.protocol, ingList)
	if err != nil {
		return elbv2model.ListenerSpec{}, err
	}
	tags, err := t.buildListenerTags(ctx, ingList)
	if err != nil {
		return elbv2model.ListenerSpec{}, err
	}
	certs := make([]elbv2model.Certificate, 0, len(config.tlsCerts))
	for _, certARN := range config.tlsCerts {
		certs = append(certs, elbv2model.Certificate{
			CertificateARN: awssdk.String(certARN),
		})
	}
	return elbv2model.ListenerSpec{
		LoadBalancerARN:      lbARN,
		Port:                 port,
		Protocol:             config.protocol,
		DefaultActions:       defaultActions,
		Certificates:         certs,
		SSLPolicy:            config.sslPolicy,
		MutualAuthentication: config.mutualAuthentication,
		Tags:                 tags,
	}, nil
}

func (t *defaultModelBuildTask) buildListenerDefaultActions(ctx context.Context, protocol elbv2model.Protocol, ingList []ClassifiedIngress) ([]elbv2model.Action, error) {
	if t.sslRedirectConfig != nil && protocol == elbv2model.ProtocolHTTP {
		return []elbv2model.Action{t.buildSSLRedirectAction(ctx, *t.sslRedirectConfig)}, nil
	}

	ingsWithDefaultBackend := make([]ClassifiedIngress, 0, len(ingList))
	for _, ing := range ingList {
		if ing.Ing.Spec.DefaultBackend != nil {
			ingsWithDefaultBackend = append(ingsWithDefaultBackend, ing)
		}
	}
	if len(ingsWithDefaultBackend) == 0 {
		action404 := t.build404Action(ctx)
		return []elbv2model.Action{action404}, nil
	}
	if len(ingsWithDefaultBackend) > 1 {
		ingKeys := make([]types.NamespacedName, 0, len(ingsWithDefaultBackend))
		for _, ing := range ingsWithDefaultBackend {
			ingKeys = append(ingKeys, k8s.NamespacedName(ing.Ing))
		}
		return nil, errors.Errorf("multiple ingress defined default backend: %v", ingKeys)
	}
	ing := ingsWithDefaultBackend[0]
	enhancedBackend, err := t.enhancedBackendBuilder.Build(ctx, ing.Ing, *ing.Ing.Spec.DefaultBackend,
		WithLoadBackendServices(true, t.backendServices),
		WithLoadAuthConfig(true))
	if err != nil {
		return nil, err
	}
	return t.buildActions(ctx, protocol, ing, enhancedBackend)
}

func (t *defaultModelBuildTask) buildListenerTags(_ context.Context, ingList []ClassifiedIngress) (map[string]string, error) {
	ingGroupTags, err := t.buildIngressGroupResourceTags(ingList)
	if err != nil {
		return nil, err
	}
	return algorithm.MergeStringMap(t.defaultTags, ingGroupTags), nil
}

// the listen port config for specific listener port.
type listenPortConfig struct {
	protocol             elbv2model.Protocol
	inboundCIDRv4s       []string
	inboundCIDRv6s       []string
	prefixLists          []string
	sslPolicy            *string
	tlsCerts             []string
	mutualAuthentication *elbv2model.MutualAuthenticationAttributes
}

func (t *defaultModelBuildTask) computeIngressListenPortConfigByPort(ctx context.Context, ing *ClassifiedIngress) (map[int64]listenPortConfig, error) {
	explicitTLSCertARNs := t.computeIngressExplicitTLSCertARNs(ctx, ing)
	explicitSSLPolicy := t.computeIngressExplicitSSLPolicy(ctx, ing)
	var prefixListIDs []string
	t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixSecurityGroupPrefixLists, &prefixListIDs, ing.Ing.Annotations)
	inboundCIDRv4s, inboundCIDRV6s, err := t.computeIngressExplicitInboundCIDRs(ctx, ing)
	if err != nil {
		return nil, err
	}
	mutualAuthenticationAttributes, err := t.computeIngressMutualAuthentication(ctx, ing)
	if err != nil {
		return nil, err
	}
	preferTLS := len(explicitTLSCertARNs) != 0
	listenPorts, err := t.computeIngressListenPorts(ctx, ing.Ing, preferTLS)
	if err != nil {
		return nil, err
	}

	containsHTTPSPort := false
	for _, protocol := range listenPorts {
		if protocol == elbv2model.ProtocolHTTPS {
			containsHTTPSPort = true
			break
		}
	}
	var inferredTLSCertARNs []string
	if containsHTTPSPort && len(explicitTLSCertARNs) == 0 {
		inferredTLSCertARNs, err = t.computeIngressInferredTLSCertARNs(ctx, ing.Ing)
		if err != nil {
			return nil, err
		}
	}

	listenPortConfigByPort := make(map[int64]listenPortConfig, len(listenPorts))
	for port, protocol := range listenPorts {
		cfg := listenPortConfig{
			protocol:       protocol,
			inboundCIDRv4s: inboundCIDRv4s,
			inboundCIDRv6s: inboundCIDRV6s,
			prefixLists:    prefixListIDs,
		}
		if protocol == elbv2model.ProtocolHTTPS {
			if len(explicitTLSCertARNs) == 0 {
				cfg.tlsCerts = inferredTLSCertARNs
			} else {
				cfg.tlsCerts = explicitTLSCertARNs
			}
			cfg.sslPolicy = explicitSSLPolicy
			cfg.mutualAuthentication = mutualAuthenticationAttributes[port]
		}
		listenPortConfigByPort[port] = cfg
	}

	return listenPortConfigByPort, nil
}

func (t *defaultModelBuildTask) computeIngressExplicitTLSCertARNs(_ context.Context, ing *ClassifiedIngress) []string {
	if ing.IngClassConfig.IngClassParams != nil && len(ing.IngClassConfig.IngClassParams.Spec.CertificateArn) != 0 {
		return ing.IngClassConfig.IngClassParams.Spec.CertificateArn
	}
	var rawTLSCertARNs []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixCertificateARN, &rawTLSCertARNs, ing.Ing.Annotations)
	return rawTLSCertARNs
}

func (t *defaultModelBuildTask) computeIngressInferredTLSCertARNs(ctx context.Context, ing *networking.Ingress) ([]string, error) {
	hosts := sets.NewString()
	for _, r := range ing.Spec.Rules {
		if len(r.Host) != 0 {
			hosts.Insert(r.Host)
		}
	}
	for _, t := range ing.Spec.TLS {
		hosts.Insert(t.Hosts...)
	}
	return t.certDiscovery.Discover(ctx, hosts.List())
}

func (t *defaultModelBuildTask) computeIngressListenPorts(_ context.Context, ing *networking.Ingress, preferTLS bool) (map[int64]elbv2model.Protocol, error) {
	rawListenPorts := ""
	if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixListenPorts, &rawListenPorts, ing.Annotations); !exists {
		if preferTLS {
			return map[int64]elbv2model.Protocol{443: elbv2model.ProtocolHTTPS}, nil
		}
		return map[int64]elbv2model.Protocol{80: elbv2model.ProtocolHTTP}, nil
	}

	var entries []map[string]int64
	if err := json.Unmarshal([]byte(rawListenPorts), &entries); err != nil {
		return nil, errors.Wrapf(err, "failed to parse listen-ports configuration: `%s`", rawListenPorts)
	}
	if len(entries) == 0 {
		return nil, errors.Errorf("empty listen-ports configuration: `%s`", rawListenPorts)
	}

	portAndProtocols := make(map[int64]elbv2model.Protocol, len(entries))
	for _, entry := range entries {
		for protocol, port := range entry {
			// Verify port value is valid for ALB: [1, 65535]
			if port < 1 || port > 65535 {
				return nil, errors.Errorf("listen port must be within [1, 65535]: %v", port)
			}
			switch protocol {
			case string(elbv2model.ProtocolHTTP):
				portAndProtocols[port] = elbv2model.ProtocolHTTP
			case string(elbv2model.ProtocolHTTPS):
				portAndProtocols[port] = elbv2model.ProtocolHTTPS
			default:
				return nil, errors.Errorf("listen protocol must be within [%v, %v]: %v", elbv2model.ProtocolHTTP, elbv2model.ProtocolHTTPS, protocol)
			}
		}
	}
	return portAndProtocols, nil
}

func (t *defaultModelBuildTask) computeIngressExplicitInboundCIDRs(_ context.Context, ing *ClassifiedIngress) ([]string, []string, error) {
	var rawInboundCIDRs []string
	fromIngressClassParams := false
	if ing.IngClassConfig.IngClassParams != nil && len(ing.IngClassConfig.IngClassParams.Spec.InboundCIDRs) != 0 {
		rawInboundCIDRs = ing.IngClassConfig.IngClassParams.Spec.InboundCIDRs
		fromIngressClassParams = true
	} else {
		_ = t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixInboundCIDRs, &rawInboundCIDRs, ing.Ing.Annotations)
	}

	var inboundCIDRv4s, inboundCIDRv6s []string
	for _, cidr := range rawInboundCIDRs {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			if fromIngressClassParams {
				return nil, nil, fmt.Errorf("invalid CIDR in IngressClassParams InboundCIDR %s: %w", cidr, err)
			}
			return nil, nil, fmt.Errorf("invalid %v settings on Ingress: %v: %w", annotations.IngressSuffixInboundCIDRs, k8s.NamespacedName(ing.Ing), err)
		}
		if strings.Contains(cidr, ":") {
			inboundCIDRv6s = append(inboundCIDRv6s, cidr)
		} else {
			inboundCIDRv4s = append(inboundCIDRv4s, cidr)
		}
	}
	return inboundCIDRv4s, inboundCIDRv6s, nil
}

func (t *defaultModelBuildTask) computeIngressExplicitSSLPolicy(_ context.Context, ing *ClassifiedIngress) *string {
	var rawSSLPolicy string
	if ing.IngClassConfig.IngClassParams != nil && ing.IngClassConfig.IngClassParams.Spec.SSLPolicy != "" {
		return &ing.IngClassConfig.IngClassParams.Spec.SSLPolicy
	}
	if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixSSLPolicy, &rawSSLPolicy, ing.Ing.Annotations); !exists {
		return nil
	}
	return &rawSSLPolicy
}

type MutualAuthenticationConfig struct {
	Port                          int64   `json:"port"`
	Mode                          string  `json:"mode"`
	TrustStore                    *string `json:"trustStore,omitempty"`
	IgnoreClientCertificateExpiry *bool   `json:"ignoreClientCertificateExpiry,omitempty"`
}

func (t *defaultModelBuildTask) computeIngressMutualAuthentication(ctx context.Context, ing *ClassifiedIngress) (map[int64]*elbv2model.MutualAuthenticationAttributes, error) {
	var rawMtlsConfigString string
	if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixMutualAuthentication, &rawMtlsConfigString, ing.Ing.Annotations); !exists {
		return nil, nil
	}

	var ingressAnnotationEntries []MutualAuthenticationConfig

	if err := json.Unmarshal([]byte(rawMtlsConfigString), &ingressAnnotationEntries); err != nil {
		return nil, errors.Wrapf(err, "failed to parse mutualAuthentication configuration from ingress annotation: `%s`", rawMtlsConfigString)
	}
	if len(ingressAnnotationEntries) == 0 {
		return nil, errors.Errorf("empty mutualAuthentication configuration from ingress annotation: `%s`", rawMtlsConfigString)
	}
	portAndMtlsAttributesMap, err := t.parseMtlsConfigEntries(ctx, ingressAnnotationEntries)
	if err != nil {
		return nil, err
	}

	parsedPortAndMtlsAttributes, err := t.parseMtlsAttributesForTrustStoreNames(ctx, portAndMtlsAttributesMap)
	if err != nil {
		return nil, err
	}
	return parsedPortAndMtlsAttributes, nil
}

func (t *defaultModelBuildTask) parseMtlsConfigEntries(_ context.Context, entries []MutualAuthenticationConfig) (map[int64]*elbv2model.MutualAuthenticationAttributes, error) {
	portAndMtlsAttributes := make(map[int64]*elbv2model.MutualAuthenticationAttributes, len(entries))

	for _, mutualAuthenticationConfig := range entries {
		port := mutualAuthenticationConfig.Port
		mode := mutualAuthenticationConfig.Mode
		truststoreNameOrArn := awssdk.StringValue(mutualAuthenticationConfig.TrustStore)
		ignoreClientCert := mutualAuthenticationConfig.IgnoreClientCertificateExpiry

		err := t.validateMutualAuthenticationConfig(port, mode, truststoreNameOrArn, ignoreClientCert)
		if err != nil {
			return nil, err
		}

		if mode == string(elbv2model.MutualAuthenticationVerifyMode) && ignoreClientCert == nil {
			ignoreClientCert = awssdk.Bool(false)
		}
		portAndMtlsAttributes[port] = &elbv2model.MutualAuthenticationAttributes{Mode: mode, TrustStoreArn: awssdk.String(truststoreNameOrArn), IgnoreClientCertificateExpiry: ignoreClientCert}
	}
	return portAndMtlsAttributes, nil
}

func (t *defaultModelBuildTask) validateMutualAuthenticationConfig(port int64, mode string, truststoreNameOrArn string, ignoreClientCert *bool) error {
	// Verify port value is valid for ALB: [1, 65535]
	if port < 1 || port > 65535 {
		return errors.Errorf("listen port must be within [1, 65535]: %v", port)
	}
	//Verify if the mutualAuthentication mode is not empty for a port
	if mode == "" {
		return errors.Errorf("mutualAuthentication mode cannot be empty for port %v", port)
	}
	//Verify if the mutualAuthentication mode is valid
	validMutualAuthenticationModes := []string{string(elbv2model.MutualAuthenticationOffMode), string(elbv2model.MutualAuthenticationPassthroughMode), string(elbv2model.MutualAuthenticationVerifyMode)}
	if !slices.Contains(validMutualAuthenticationModes, mode) {
		return errors.Errorf("mutualAuthentication mode value must be among [%v, %v, %v] for port %v : %s", elbv2model.MutualAuthenticationOffMode, elbv2model.MutualAuthenticationPassthroughMode, elbv2model.MutualAuthenticationVerifyMode, port, mode)
	}
	//Verify if the mutualAuthentication truststoreNameOrArn is not empty for Verify mode
	if mode == string(elbv2model.MutualAuthenticationVerifyMode) && truststoreNameOrArn == "" {
		return errors.Errorf("trustStore is required when mutualAuthentication mode is verify for port %v", port)
	}
	//Verify if the mutualAuthentication truststoreNameOrArn is empty for Off and Passthrough modes
	if (mode == string(elbv2model.MutualAuthenticationOffMode) || mode == string(elbv2model.MutualAuthenticationPassthroughMode)) && truststoreNameOrArn != "" {
		return errors.Errorf("Mutual Authentication mode %s does not support trustStore for port %v", mode, port)
	}
	//Verify if the mutualAuthentication ignoreClientCert is valid for Off and Passthrough modes
	if (mode == string(elbv2model.MutualAuthenticationOffMode) || mode == string(elbv2model.MutualAuthenticationPassthroughMode)) && ignoreClientCert != nil {
		return errors.Errorf("Mutual Authentication mode %s does not support ignoring client certificate expiry for port %v", mode, port)
	}

	return nil
}

func (t *defaultModelBuildTask) parseMtlsAttributesForTrustStoreNames(ctx context.Context, portAndMtlsAttributes map[int64]*elbv2model.MutualAuthenticationAttributes) (map[int64]*elbv2model.MutualAuthenticationAttributes, error) {
	var trustStoreNames []string
	trustStoreNameAndPortMap := make(map[string][]int64)

	for port, attributes := range portAndMtlsAttributes {
		mode := attributes.Mode
		truststoreNameOrArn := awssdk.StringValue(attributes.TrustStoreArn)
		if mode == string(elbv2model.MutualAuthenticationVerifyMode) && !strings.HasPrefix(truststoreNameOrArn, "arn:") {
			trustStoreNameAndPortMap[truststoreNameOrArn] = append(trustStoreNameAndPortMap[truststoreNameOrArn], port)
		}
	}

	if len(trustStoreNameAndPortMap) != 0 {
		for names := range trustStoreNameAndPortMap {
			trustStoreNames = append(trustStoreNames, names)
		}
		tsNameAndArnMap, err := t.fetchTrustStoreArnFromName(ctx, trustStoreNames)
		if err != nil {
			return nil, err
		}
		for name, ports := range trustStoreNameAndPortMap {
			for _, port := range ports {
				attributes := portAndMtlsAttributes[port]
				if awssdk.StringValue(attributes.TrustStoreArn) != "" {
					attributes.TrustStoreArn = tsNameAndArnMap[name]
				}
				portAndMtlsAttributes[port] = attributes
			}
		}
	}
	return portAndMtlsAttributes, nil
}

func (t *defaultModelBuildTask) fetchTrustStoreArnFromName(ctx context.Context, trustStoreNames []string) (map[string]*string, error) {
	tsNameAndArnMap := make(map[string]*string, len(trustStoreNames))
	req := &elbv2sdk.DescribeTrustStoresInput{
		Names: awssdk.StringSlice(trustStoreNames),
	}
	trustStores, err := t.elbv2Client.DescribeTrustStoresWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(trustStores.TrustStores) == 0 {
		return nil, errors.Errorf("couldn't find TrustStore with names %v", trustStoreNames)
	}
	for _, tsName := range trustStoreNames {
		for _, ts := range trustStores.TrustStores {
			if tsName == awssdk.StringValue(ts.Name) {
				tsNameAndArnMap[tsName] = ts.TrustStoreArn
			}
		}
	}
	for _, tsName := range trustStoreNames {
		_, exists := tsNameAndArnMap[tsName]
		if !exists {
			return nil, errors.Errorf("couldn't find TrustStore with name %v", tsName)
		}
	}
	return tsNameAndArnMap, nil
}
