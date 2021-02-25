package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"net"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strings"
)

func (t *defaultModelBuildTask) buildListener(ctx context.Context, lbARN core.StringToken, port int64, config listenPortConfig, ingList []*networking.Ingress) (*elbv2model.Listener, error) {
	lsSpec, err := t.buildListenerSpec(ctx, lbARN, port, config, ingList)
	if err != nil {
		return nil, err
	}
	lsResID := fmt.Sprintf("%v", port)
	ls := elbv2model.NewListener(t.stack, lsResID, lsSpec)
	return ls, nil
}

func (t *defaultModelBuildTask) buildListenerSpec(ctx context.Context, lbARN core.StringToken, port int64, config listenPortConfig, ingList []*networking.Ingress) (elbv2model.ListenerSpec, error) {
	defaultActions, err := t.buildListenerDefaultActions(ctx, config.protocol, ingList)
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
		LoadBalancerARN: lbARN,
		Port:            port,
		Protocol:        config.protocol,
		DefaultActions:  defaultActions,
		Certificates:    certs,
		SSLPolicy:       config.sslPolicy,
	}, nil
}

func (t *defaultModelBuildTask) buildListenerDefaultActions(ctx context.Context, protocol elbv2model.Protocol, ingList []*networking.Ingress) ([]elbv2model.Action, error) {
	if t.sslRedirectConfig != nil && protocol == elbv2model.ProtocolHTTP {
		return []elbv2model.Action{t.buildSSLRedirectAction(ctx, *t.sslRedirectConfig)}, nil
	}

	ingsWithDefaultBackend := make([]*networking.Ingress, 0, len(ingList))
	for _, ing := range ingList {
		if ing.Spec.Backend != nil {
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
			ingKeys = append(ingKeys, k8s.NamespacedName(ing))
		}
		return nil, errors.Errorf("multiple ingress defined default backend: %v", ingKeys)
	}
	ing := ingsWithDefaultBackend[0]
	enhancedBackend, err := t.enhancedBackendBuilder.Build(ctx, ing, *ing.Spec.Backend)
	if err != nil {
		return nil, err
	}
	return t.buildActions(ctx, protocol, ing, enhancedBackend)
}

// the listen port config for specific Ingress's port
type listenPortConfig struct {
	protocol       elbv2model.Protocol
	inboundCIDRv4s []string
	inboundCIDRv6s []string
	sslPolicy      *string
	tlsCerts       []string
}

func (t *defaultModelBuildTask) computeIngressListenPortConfigByPort(ctx context.Context, ing *networking.Ingress) (map[int64]listenPortConfig, error) {
	explicitTLSCertARNs := t.computeIngressExplicitTLSCertARNs(ctx, ing)
	explicitSSLPolicy := t.computeIngressExplicitSSLPolicy(ctx, ing)
	inboundCIDRv4s, inboundCIDRV6s, err := t.computeIngressExplicitInboundCIDRs(ctx, ing)
	if err != nil {
		return nil, err
	}
	preferTLS := len(explicitTLSCertARNs) != 0
	listenPorts, err := t.computeIngressListenPorts(ctx, ing, preferTLS)
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
		inferredTLSCertARNs, err = t.computeIngressInferredTLSCertARNs(ctx, ing)
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
		}
		if protocol == elbv2model.ProtocolHTTPS {
			if len(explicitTLSCertARNs) == 0 {
				cfg.tlsCerts = inferredTLSCertARNs
			} else {
				cfg.tlsCerts = explicitTLSCertARNs
			}
			cfg.sslPolicy = explicitSSLPolicy
		}
		listenPortConfigByPort[port] = cfg
	}

	return listenPortConfigByPort, nil
}

func (t *defaultModelBuildTask) computeIngressExplicitTLSCertARNs(_ context.Context, ing *networking.Ingress) []string {
	var rawTLSCertARNs []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixCertificateARN, &rawTLSCertARNs, ing.Annotations)
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

func (t *defaultModelBuildTask) computeIngressExplicitInboundCIDRs(_ context.Context, ing *networking.Ingress) ([]string, []string, error) {
	var rawInboundCIDRs []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixInboundCIDRs, &rawInboundCIDRs, ing.Annotations)

	var inboundCIDRv4s, inboundCIDRv6s []string
	for _, cidr := range rawInboundCIDRs {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "invalid %v settings on Ingress: %v", annotations.IngressSuffixInboundCIDRs, k8s.NamespacedName(ing))
		}
		if strings.Contains(cidr, ":") {
			inboundCIDRv6s = append(inboundCIDRv6s, cidr)
		} else {
			inboundCIDRv4s = append(inboundCIDRv4s, cidr)
		}
	}
	return inboundCIDRv4s, inboundCIDRv6s, nil
}

func (t *defaultModelBuildTask) computeIngressExplicitSSLPolicy(_ context.Context, ing *networking.Ingress) *string {
	var rawSSLPolicy string
	if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixSSLPolicy, &rawSSLPolicy, ing.Annotations); !exists {
		return nil
	}
	return &rawSSLPolicy
}
