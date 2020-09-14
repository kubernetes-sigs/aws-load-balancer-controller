package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
)

func (b *defaultModelBuilder) buildListenerAndListenerRules(ctx context.Context, stack core.Stack, ingGroup Group, lbARN core.StringToken) error {
	protocolByPort := make(map[int64]elbv2model.Protocol)
	sslPolicyByPort := make(map[int64]string)
	tlsCertARNsByPort := make(map[int64][]string)
	defaultActionsByPort := make(map[int64][]elbv2model.Action)
	ingressesByPort := make(map[int64][]*networking.Ingress)
	tgByID := make(map[string]*elbv2model.TargetGroup)
	for _, ing := range ingGroup.Members {
		explicitSSLPolicy := b.buildExplicitSSLPolicyForIngress(ctx, ing)
		explicitTLSCerts := b.buildExplicitTLSCertARNsForIngress(ctx, ing)
		portAndProtocols, err := b.buildPortAndProtocolsForIngress(ctx, ing, len(explicitTLSCerts) != 0)
		if err != nil {
			return err
		}
		for port, protocol := range portAndProtocols {
			if existingProtocol, exists := protocolByPort[port]; !exists {
				protocolByPort[port] = protocol
			} else if existingProtocol != protocol {
				return errors.Errorf("conflict protocol for port %v: %v | %v", port, existingProtocol, protocol)
			}

			protocolByPort[port] = protocol
			if protocol == elbv2model.ProtocolHTTPS {
				if explicitSSLPolicy != nil {
					if existingSSLPolicy, exists := sslPolicyByPort[port]; !exists {
						sslPolicyByPort[port] = awssdk.StringValue(explicitSSLPolicy)
					} else if existingSSLPolicy != awssdk.StringValue(explicitSSLPolicy) {
						return errors.Errorf("conflict SSLPolicy for port %v: %v | %v", port, existingSSLPolicy, awssdk.StringValue(explicitSSLPolicy))
					}
				}

				// maintain original order for tlsCertsByPort[port], since we use the first cert as default listener certificate.
				existingTLSCertSet := sets.NewString(tlsCertARNsByPort[port]...)
				for _, cert := range explicitTLSCerts {
					if !existingTLSCertSet.Has(cert) {
						tlsCertARNsByPort[port] = append(tlsCertARNsByPort[port], cert)
						existingTLSCertSet.Insert(cert)
					}
				}
			}
			if ing.Spec.Backend != nil {
				if _, exists := defaultActionsByPort[port]; !exists {
					defaultActions, err := b.buildExplicitDefaultActionsForIngress(ctx, stack, ingGroup.ID, tgByID, protocol, ing)
					if err != nil {
						return err
					}
					defaultActionsByPort[port] = defaultActions
				} else {
					return errors.Errorf("at most one Ingress can specify default backend")
				}
			}
			ingressesByPort[port] = append(ingressesByPort[port], ing)
		}
	}

	for port, protocol := range protocolByPort {
		sslPolicy, exists := sslPolicyByPort[port]
		if protocol == elbv2model.ProtocolHTTPS && !exists {
			sslPolicy = b.defaultSSLPolicy
		}

		certARNs := tlsCertARNsByPort[port]
		certs := make([]elbv2model.Certificate, 0, len(certARNs))
		for _, certARN := range certARNs {
			certs = append(certs, elbv2model.Certificate{
				CertificateARN: awssdk.String(certARN),
			})
		}
		defaultActions, exists := defaultActionsByPort[port]
		if !exists {
			defaultActions = []elbv2model.Action{b.build404Action(ctx)}
		}
		lsResID := fmt.Sprintf("%v", port)
		ls := elbv2model.NewListener(stack, lsResID, elbv2model.ListenerSpec{
			LoadBalancerARN: lbARN,
			Port:            port,
			Protocol:        protocol,
			DefaultActions:  defaultActions,
			Certificates:    certs,
			SSLPolicy:       awssdk.String(sslPolicy),
		})
		if err := b.buildListenerRules(ctx, stack, ingGroup.ID, port, protocol, ls.ListenerARN(), tgByID, ingressesByPort[port]); err != nil {
			return err
		}
	}
	return nil
}

func (b *defaultModelBuilder) buildExplicitTLSCertARNsForIngress(ctx context.Context, ing *networking.Ingress) []string {
	var rawTLSCertARNs []string
	_ = b.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixCertificateARN, &rawTLSCertARNs, ing.Annotations)
	return rawTLSCertARNs
}

func (b *defaultModelBuilder) buildExplicitSSLPolicyForIngress(ctx context.Context, ing *networking.Ingress) *string {
	var rawSSLPolicy string
	if exists := b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixSSLPolicy, &rawSSLPolicy, ing.Annotations); !exists {
		return nil
	}
	return &rawSSLPolicy
}

func (b *defaultModelBuilder) buildExplicitDefaultActionsForIngress(ctx context.Context, stack core.Stack, ingGroupID GroupID,
	tgByID map[string]*elbv2model.TargetGroup, protocol elbv2model.Protocol, ing *networking.Ingress) ([]elbv2model.Action, error) {
	if ing.Spec.Backend == nil {
		return nil, nil
	}
	defaultBackend, err := b.enhancedBackendBuilder.Build(ctx, ing, *ing.Spec.Backend)
	if err != nil {
		return nil, err
	}
	return b.buildActions(ctx, stack, ingGroupID, tgByID, protocol, ing, defaultBackend)
}

func (b *defaultModelBuilder) buildPortAndProtocolsForIngress(ctx context.Context, ing *networking.Ingress, preferTLS bool) (map[int64]elbv2model.Protocol, error) {
	rawListenPorts := ""
	if exists := b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixListenPorts, &rawListenPorts, ing.Annotations); !exists {
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
