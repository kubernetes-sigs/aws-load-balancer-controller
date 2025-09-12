package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

var (
	validMultiProtocolSet = sets.New(string(elbv2model.ProtocolTCP), string(elbv2model.ProtocolUDP))
)

func (t *defaultModelBuildTask) buildListeners(ctx context.Context, scheme elbv2model.LoadBalancerScheme) error {

	if t.shouldUseTCPUDP() {
		return t.buildListenersWithTCPUDPSupport(ctx, scheme)
	}
	return t.buildListenersLegacy(ctx, scheme)
}

func (t *defaultModelBuildTask) buildListenersLegacy(ctx context.Context, scheme elbv2model.LoadBalancerScheme) error {
	cfg, err := t.buildListenerConfig(ctx, sets.New[int32]())
	if err != nil {
		return err
	}

	for _, port := range t.service.Spec.Ports {
		_, err = t.buildListener(ctx, port, cfg, scheme)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *defaultModelBuildTask) buildListenersWithTCPUDPSupport(ctx context.Context, scheme elbv2model.LoadBalancerScheme) error {
	// group by listener port number
	portMap := make(map[int32][]corev1.ServicePort)
	for _, port := range t.service.Spec.Ports {
		key := port.Port
		if vals, exists := portMap[key]; exists {
			portMap[key] = append(vals, port)
		} else {
			portMap[key] = []corev1.ServicePort{port}
		}
	}

	// Now calculate any multi-protocol usage.
	tcpUdpPortSet := sets.New[int32]()
	for _, servicePorts := range portMap {
		if len(servicePorts) > 1 {
			err := validateMultiProtocolUsage(servicePorts)
			if err != nil {
				return err
			}
			tcpUdpPortSet.Insert(servicePorts[0].Port)
		}
	}

	cfg, err := t.buildListenerConfig(ctx, tcpUdpPortSet)
	if err != nil {
		return err
	}

	// Finally, build the listeners.
	// This code loops over port map 3 times! But the port map should be relatively small.
	for _, servicePorts := range portMap {
		_, err = t.buildListener(ctx, servicePorts[0], cfg, scheme)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *defaultModelBuildTask) buildListener(ctx context.Context, port corev1.ServicePort, cfg *listenerConfig,
	scheme elbv2model.LoadBalancerScheme) (*elbv2model.Listener, error) {
	lsSpec, err := t.buildListenerSpec(ctx, port, cfg, scheme)
	if err != nil {
		return nil, err
	}
	listenerResID := fmt.Sprintf("%v", port.Port)
	ls := elbv2model.NewListener(t.stack, listenerResID, lsSpec)
	return ls, nil
}

func (t *defaultModelBuildTask) buildListenerSpec(ctx context.Context, port corev1.ServicePort, cfg *listenerConfig,
	scheme elbv2model.LoadBalancerScheme) (elbv2model.ListenerSpec, error) {

	var tgProtocol elbv2model.Protocol
	var listenerProtocol elbv2model.Protocol

	if cfg != nil && cfg.tcpUdpPortsSet.Has(port.Port) {
		tgProtocol = elbv2model.ProtocolTCP_UDP
		listenerProtocol = elbv2model.ProtocolTCP_UDP
	} else {
		tgProtocol = elbv2model.Protocol(port.Protocol)
		listenerProtocol = elbv2model.Protocol(port.Protocol)
	}

	if (tgProtocol != elbv2model.ProtocolUDP && tgProtocol != elbv2model.ProtocolTCP_UDP) && len(cfg.certificates) != 0 && (cfg.tlsPortsSet.Len() == 0 ||
		cfg.tlsPortsSet.Has(port.Name) || cfg.tlsPortsSet.Has(strconv.Itoa(int(port.Port)))) {
		if cfg.backendProtocol == "ssl" {
			tgProtocol = elbv2model.ProtocolTLS
		}
		listenerProtocol = elbv2model.ProtocolTLS
	}

	if cfg != nil && (cfg.quicPortsSet.Has(port.Name) || cfg.quicPortsSet.Has(strconv.Itoa(int(port.Port)))) {
		switch listenerProtocol {
		case elbv2model.ProtocolUDP:
			tgProtocol = elbv2model.ProtocolQUIC
			listenerProtocol = elbv2model.ProtocolQUIC
			break
		case elbv2model.ProtocolTCP_UDP:
			tgProtocol = elbv2model.ProtocolTCP_QUIC
			listenerProtocol = elbv2model.ProtocolTCP_QUIC
			break
		default:
			return elbv2model.ListenerSpec{}, errors.Errorf("Unsupported QUIC upgrade for protocol %v", listenerProtocol)
		}
	}

	tags, err := t.buildListenerTags(ctx)
	if err != nil {
		return elbv2model.ListenerSpec{}, err
	}
	targetGroup, err := t.buildTargetGroup(ctx, port, tgProtocol, scheme)
	if err != nil {
		return elbv2model.ListenerSpec{}, err
	}

	alpnPolicy, err := t.buildListenerALPNPolicy(ctx, listenerProtocol, tgProtocol)
	if err != nil {
		return elbv2model.ListenerSpec{}, err
	}

	var sslPolicy *string
	var certificates []elbv2model.Certificate
	if listenerProtocol == elbv2model.ProtocolTLS {
		sslPolicy = cfg.sslPolicy
		certificates = cfg.certificates
	}

	defaultActions := t.buildListenerDefaultActions(ctx, targetGroup)
	lsAttributes, attributesErr := t.buildListenerAttributes(ctx, t.service.Annotations, port.Port, listenerProtocol)
	if attributesErr != nil {
		return elbv2model.ListenerSpec{}, attributesErr
	}
	return elbv2model.ListenerSpec{
		LoadBalancerARN:    t.loadBalancer.LoadBalancerARN(),
		Port:               port.Port,
		Protocol:           listenerProtocol,
		Certificates:       certificates,
		SSLPolicy:          sslPolicy,
		ALPNPolicy:         alpnPolicy,
		DefaultActions:     defaultActions,
		Tags:               tags,
		ListenerAttributes: lsAttributes,
	}, nil
}

func (t *defaultModelBuildTask) buildListenerDefaultActions(_ context.Context, targetGroup *elbv2model.TargetGroup) []elbv2model.Action {
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

func (t *defaultModelBuildTask) buildSSLNegotiationPolicy(_ context.Context) *string {
	rawSslPolicyStr := ""
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixSSLNegotiationPolicy, &rawSslPolicyStr, t.service.Annotations); exists {
		return &rawSslPolicyStr
	}
	return &t.defaultSSLPolicy
}

func (t *defaultModelBuildTask) buildListenerCertificates(_ context.Context) []elbv2model.Certificate {
	var rawCertificateARNs []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLCertificate, &rawCertificateARNs, t.service.Annotations)

	var certificates []elbv2model.Certificate
	for _, cert := range rawCertificateARNs {
		certificates = append(certificates, elbv2model.Certificate{CertificateARN: aws.String(cert)})
	}
	return certificates
}

func validateTLSPortsSet(rawTLSPorts []string, ports []corev1.ServicePort) error {
	unusedPorts := make([]string, 0)

	for _, tlsPort := range rawTLSPorts {
		isPortUsed := false
		for _, portObj := range ports {
			if portObj.Name == tlsPort || strconv.Itoa(int(portObj.Port)) == tlsPort {
				isPortUsed = true
				break
			}
		}

		if !isPortUsed {
			unusedPorts = append(unusedPorts, tlsPort)
		}
	}

	if len(unusedPorts) > 0 {
		unusedPortErr := errors.Errorf("Unused port in ssl-ports annotation %v", unusedPorts)
		return unusedPortErr
	}

	return nil
}

func (t *defaultModelBuildTask) buildTLSPortsSet(_ context.Context) (sets.Set[string], error) {
	var rawTLSPorts []string

	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLPorts, &rawTLSPorts, t.service.Annotations)

	err := validateTLSPortsSet(rawTLSPorts, t.service.Spec.Ports)

	if err != nil {
		return nil, err
	}

	return sets.New[string](rawTLSPorts...), nil
}

func (t *defaultModelBuildTask) buildQUICPortsSet() sets.Set[string] {
	var rawQUICPorts []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixQUICEnabledPorts, &rawQUICPorts, t.service.Annotations)
	return sets.New[string](rawQUICPorts...)
}

func (t *defaultModelBuildTask) buildBackendProtocol(_ context.Context) string {
	rawBackendProtocol := ""
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixBEProtocol, &rawBackendProtocol, t.service.Annotations)
	return rawBackendProtocol
}

func (t *defaultModelBuildTask) buildListenerALPNPolicy(ctx context.Context, listenerProtocol elbv2model.Protocol,
	targetGroupProtocol elbv2model.Protocol) ([]string, error) {
	if listenerProtocol != elbv2model.ProtocolTLS {
		return nil, nil
	}
	var rawALPNPolicy string
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixALPNPolicy, &rawALPNPolicy, t.service.Annotations); !exists {
		return nil, nil
	}
	switch elbv2model.ALPNPolicy(rawALPNPolicy) {
	case elbv2model.ALPNPolicyNone, elbv2model.ALPNPolicyHTTP1Only, elbv2model.ALPNPolicyHTTP2Only,
		elbv2model.ALPNPolicyHTTP2Preferred, elbv2model.ALPNPolicyHTTP2Optional:
		return []string{rawALPNPolicy}, nil
	default:
		return nil, errors.Errorf("invalid ALPN policy %v, policy must be one of [%v, %v, %v, %v, %v]",
			rawALPNPolicy, elbv2model.ALPNPolicyNone, elbv2model.ALPNPolicyHTTP1Only, elbv2model.ALPNPolicyHTTP2Only,
			elbv2model.ALPNPolicyHTTP2Optional, elbv2model.ALPNPolicyHTTP2Preferred)
	}
}

type listenerConfig struct {
	certificates    []elbv2model.Certificate
	tlsPortsSet     sets.Set[string]
	quicPortsSet    sets.Set[string]
	tcpUdpPortsSet  sets.Set[int32]
	sslPolicy       *string
	backendProtocol string
}

func (t *defaultModelBuildTask) buildListenerConfig(ctx context.Context, tcpUdpPortsSet sets.Set[int32]) (*listenerConfig, error) {
	certificates := t.buildListenerCertificates(ctx)
	tlsPortsSet, err := t.buildTLSPortsSet(ctx)
	if err != nil {
		return nil, err
	}

	quicPortsSets := t.buildQUICPortsSet()

	backendProtocol := t.buildBackendProtocol(ctx)
	sslPolicy := t.buildSSLNegotiationPolicy(ctx)

	return &listenerConfig{
		certificates:    certificates,
		tlsPortsSet:     tlsPortsSet,
		quicPortsSet:    quicPortsSets,
		tcpUdpPortsSet:  tcpUdpPortsSet,
		sslPolicy:       sslPolicy,
		backendProtocol: backendProtocol,
	}, nil
}

func (t *defaultModelBuildTask) buildListenerTags(ctx context.Context) (map[string]string, error) {
	return t.buildAdditionalResourceTags(ctx)
}

// Build attributes for listener
func (t *defaultModelBuildTask) buildListenerAttributes(ctx context.Context, svcAnnotations map[string]string, port int32, listenerProtocol elbv2model.Protocol) ([]elbv2model.ListenerAttribute, error) {
	var rawAttributes map[string]string
	annotationKey := fmt.Sprintf("%v.%v-%v", annotations.SvcLBSuffixlsAttsAnnotationPrefix, listenerProtocol, port)
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotationKey, &rawAttributes, svcAnnotations); err != nil {
		return nil, err
	}
	attributes := make([]elbv2model.ListenerAttribute, 0, len(rawAttributes))
	for attrKey, attrValue := range rawAttributes {
		attributes = append(attributes, elbv2model.ListenerAttribute{
			Key:   attrKey,
			Value: attrValue,
		})
	}
	return attributes, nil
}

func validateMultiProtocolUsage(ports []corev1.ServicePort) error {
	if len(ports) != 2 {
		return fmt.Errorf("can only merge two ports, not %d (%+v)", len(ports), ports)
	}
	for _, port := range ports {
		if !validMultiProtocolSet.Has(string(port.Protocol)) {
			return fmt.Errorf("unsupported protocol for merging: %s", port.Protocol)
		}
	}
	if ports[0].Protocol == ports[1].Protocol {
		return fmt.Errorf("protocols can't match for merging: %s", ports[0].Protocol)
	}
	return nil
}

func (t *defaultModelBuildTask) shouldUseTCPUDP() bool {
	annotationValue := t.isTCPUDPEnabledForService(t.service.Annotations)

	if annotationValue != nil {
		return *annotationValue
	}
	return t.enableTCPUDPSupport
}

func (t *defaultModelBuildTask) isTCPUDPEnabledForService(svcAnnotations map[string]string) *bool {
	var rawEnabled bool
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixEnableTCPUDPListener, &rawEnabled, svcAnnotations)
	if !exists || err != nil {
		return nil
	}
	return &rawEnabled
}
