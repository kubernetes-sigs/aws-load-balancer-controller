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

func (t *defaultModelBuildTask) buildListeners(ctx context.Context, scheme elbv2model.LoadBalancerScheme) error {
	cfg, err := t.buildListenerConfig(ctx)
	if err != nil {
		return err
	}

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

	// execute build listener
	for _, port := range t.service.Spec.Ports {
		key := port.Port
		if vals, exists := portMap[key]; exists {
			if len(vals) > 1 {
				port = mergeServicePortsForListener(vals)
			} else {
				port = vals[0]
			}
			_, err := t.buildListener(ctx, port, cfg, scheme)
			if err != nil {
				return err
			}
			delete(portMap, key)
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
	tgProtocol := elbv2model.Protocol(port.Protocol)
	listenerProtocol := elbv2model.Protocol(port.Protocol)
	if tgProtocol != elbv2model.ProtocolUDP && len(cfg.certificates) != 0 && (cfg.tlsPortsSet.Len() == 0 ||
		cfg.tlsPortsSet.Has(port.Name) || cfg.tlsPortsSet.Has(strconv.Itoa(int(port.Port)))) {
		if cfg.backendProtocol == "ssl" {
			tgProtocol = elbv2model.ProtocolTLS
		}
		listenerProtocol = elbv2model.ProtocolTLS
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
	sslPolicy       *string
	backendProtocol string
}

func (t *defaultModelBuildTask) buildListenerConfig(ctx context.Context) (*listenerConfig, error) {
	certificates := t.buildListenerCertificates(ctx)
	tlsPortsSet, err := t.buildTLSPortsSet(ctx)
	if err != nil {
		return nil, err
	}

	backendProtocol := t.buildBackendProtocol(ctx)
	sslPolicy := t.buildSSLNegotiationPolicy(ctx)

	return &listenerConfig{
		certificates:    certificates,
		tlsPortsSet:     tlsPortsSet,
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

func mergeServicePortsForListener(ports []corev1.ServicePort) corev1.ServicePort {
	port0 := ports[0]
	mergeableProtocols := map[corev1.Protocol]bool{
		corev1.ProtocolTCP: true,
		corev1.ProtocolUDP: true,
	}
	if _, ok := mergeableProtocols[port0.Protocol]; !ok {
		return port0
	}
	for _, port := range ports[1:] {
		if _, ok := mergeableProtocols[port.Protocol]; !ok {
			continue
		}
		if port.NodePort == port0.NodePort && port.Protocol != port0.Protocol {
			port0.Protocol = corev1.Protocol("TCP_UDP")
			break
		}
	}
	return port0
}
