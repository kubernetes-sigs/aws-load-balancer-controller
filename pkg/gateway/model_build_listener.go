package gateway

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

func (t *defaultModelBuildTask) buildListeners(ctx context.Context, scheme elbv2model.LoadBalancerScheme) error {
	cfg, err := t.buildListenerConfig(ctx)
	if err != nil {
		return err
	}

	for _, listener := range t.gateway.Spec.Listeners {
		_, err := t.buildListener(ctx, listener, *cfg, scheme)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *defaultModelBuildTask) buildListener(ctx context.Context, listener v1beta1.Listener, cfg listenerConfig,
	scheme elbv2model.LoadBalancerScheme) (*elbv2model.Listener, error) {
	lsSpec, err := t.buildListenerSpec(ctx, listener, cfg, scheme)
	if err != nil {
		return nil, err
	}
	listenerResID := fmt.Sprintf("%v", listener.Port)
	ls := elbv2model.NewListener(t.stack, listenerResID, lsSpec)
	return ls, nil
}

func (t *defaultModelBuildTask) buildListenerSpec(ctx context.Context, listener v1beta1.Listener, cfg listenerConfig,
	scheme elbv2model.LoadBalancerScheme) (elbv2model.ListenerSpec, error) {
	tgProtocol := elbv2model.Protocol(listener.Protocol)
	listenerProtocol := elbv2model.Protocol(listener.Protocol)
	if tgProtocol != elbv2model.ProtocolUDP && len(cfg.certificates) != 0 && (cfg.tlsPortsSet.Len() == 0 ||
		listener.Protocol == v1beta1.HTTPSProtocolType || listener.Protocol == v1beta1.TLSProtocolType ||
		cfg.tlsPortsSet.Has(strconv.Itoa(int(listener.Port)))) {
		if cfg.backendProtocol == "ssl" {
			tgProtocol = elbv2model.ProtocolTLS
		}
		listenerProtocol = elbv2model.ProtocolTLS
	}

	tags, err := t.buildListenerTags(ctx)
	if err != nil {
		return elbv2model.ListenerSpec{}, err
	}
	targetGroup, err := t.buildTargetGroup(ctx, &listener, tgProtocol, scheme)
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
	return elbv2model.ListenerSpec{
		LoadBalancerARN: t.loadBalancer.LoadBalancerARN(),
		Port:            int64(listener.Port),
		Protocol:        listenerProtocol,
		Certificates:    certificates,
		SSLPolicy:       sslPolicy,
		ALPNPolicy:      alpnPolicy,
		DefaultActions:  defaultActions,
		Tags:            tags,
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
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixSSLNegotiationPolicy, &rawSslPolicyStr, t.gateway.Annotations); exists {
		return &rawSslPolicyStr
	}
	return &t.defaultSSLPolicy
}

func (t *defaultModelBuildTask) buildListenerCertificates(_ context.Context) []elbv2model.Certificate {
	var rawCertificateARNs []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLCertificate, &rawCertificateARNs, t.gateway.Annotations)

	var certificates []elbv2model.Certificate
	for _, cert := range rawCertificateARNs {
		certificates = append(certificates, elbv2model.Certificate{CertificateARN: aws.String(cert)})
	}
	return certificates
}

func validateTLSPortsSet(rawTLSPorts []string, listeners []v1beta1.Listener) error {
	unusedPorts := make([]string, 0)

	for _, tlsPort := range rawTLSPorts {
		isPortUsed := false
		for _, listener := range listeners {
			if string(listener.Name) == tlsPort || strconv.Itoa(int(listener.Port)) == tlsPort {
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

func (t *defaultModelBuildTask) buildTLSPortsSet(_ context.Context) (sets.String, error) {
	var rawTLSPorts []string

	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLPorts, &rawTLSPorts, t.gateway.Annotations)

	err := validateTLSPortsSet(rawTLSPorts, t.gateway.Spec.Listeners)

	if err != nil {
		return nil, err
	}

	return sets.NewString(rawTLSPorts...), nil
}

func (t *defaultModelBuildTask) buildBackendProtocol(_ context.Context) string {
	rawBackendProtocol := ""
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixBEProtocol, &rawBackendProtocol, t.gateway.Annotations)
	return rawBackendProtocol
}

func (t *defaultModelBuildTask) buildListenerALPNPolicy(ctx context.Context, listenerProtocol elbv2model.Protocol,
	targetGroupProtocol elbv2model.Protocol) ([]string, error) {
	if listenerProtocol != elbv2model.ProtocolTLS {
		return nil, nil
	}
	var rawALPNPolicy string
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixALPNPolicy, &rawALPNPolicy, t.gateway.Annotations); !exists {
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
	tlsPortsSet     sets.String
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
