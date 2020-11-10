package service

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strconv"
)

func (t *defaultModelBuildTask) buildListeners(ctx context.Context) error {
	cfg := t.buildListenerConfig(ctx)
	for _, port := range t.service.Spec.Ports {
		_, err := t.buildListener(ctx, port, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *defaultModelBuildTask) buildListener(ctx context.Context, port corev1.ServicePort, cfg listenerConfig) (*elbv2model.Listener, error) {
	lsSpec, err := t.buildListenerSpec(ctx, port, cfg)
	if err != nil {
		return nil, err
	}
	listenerResID := fmt.Sprintf("%v", port.Port)
	ls := elbv2model.NewListener(t.stack, listenerResID, lsSpec)
	return ls, nil
}

func (t *defaultModelBuildTask) buildListenerSpec(ctx context.Context, port corev1.ServicePort, cfg listenerConfig) (elbv2model.ListenerSpec, error) {
	tgProtocol := elbv2model.Protocol(port.Protocol)
	listenerProtocol := elbv2model.Protocol(port.Protocol)
	if tgProtocol != elbv2model.ProtocolUDP && cfg.certificateARNs != nil && (cfg.tlsPortsSet.Len() == 0 ||
		cfg.tlsPortsSet.Has(port.Name) || cfg.tlsPortsSet.Has(strconv.Itoa(int(port.Port)))) {
		if cfg.backendProtocol == "ssl" {
			tgProtocol = elbv2model.ProtocolTLS
		}
		listenerProtocol = elbv2model.ProtocolTLS
	}

	targetGroup, err := t.buildTargetGroup(ctx, port, tgProtocol)
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
		Port:            int64(port.Port),
		Protocol:        listenerProtocol,
		Certificates:    certificates,
		SSLPolicy:       sslPolicy,
		DefaultActions:  defaultActions,
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
	var sslPolicy *string = nil
	rawSslPolicyStr := ""
	exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixSSLNegotiationPolicy, &rawSslPolicyStr, t.service.Annotations)
	if exists {
		sslPolicy = &rawSslPolicyStr
	}
	return sslPolicy
}

func (t *defaultModelBuildTask) buildListenerCertificateARNs(_ context.Context) []string {
	var rawCertificateARNs []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLCertificate, &rawCertificateARNs, t.service.Annotations)
	return rawCertificateARNs
}

func (t *defaultModelBuildTask) buildTLSPortsSet(_ context.Context) sets.String {
	var rawTLSPorts []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLPorts, &rawTLSPorts, t.service.Annotations)
	return sets.NewString(rawTLSPorts...)
}

func (t *defaultModelBuildTask) buildBackendProtocol(_ context.Context) string {
	rawBackendProtocol := ""
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixBEProtocol, &rawBackendProtocol, t.service.Annotations)
	return rawBackendProtocol
}

type listenerConfig struct {
	certificateARNs []string
	certificates    []elbv2model.Certificate
	tlsPortsSet     sets.String
	sslPolicy       *string
	backendProtocol string
}

func (t *defaultModelBuildTask) buildListenerConfig(ctx context.Context) listenerConfig {
	certificateARNs := t.buildListenerCertificateARNs(ctx)
	tlsPortsSet := t.buildTLSPortsSet(ctx)
	backendProtocol := t.buildBackendProtocol(ctx)
	sslPolicy := t.buildSSLNegotiationPolicy(ctx)
	var certificates []elbv2model.Certificate

	for _, cert := range certificateARNs {
		certificates = append(certificates, elbv2model.Certificate{CertificateARN: aws.String(cert)})
	}
	return listenerConfig{
		certificateARNs: certificateARNs,
		certificates:    certificates,
		tlsPortsSet:     tlsPortsSet,
		sslPolicy:       sslPolicy,
		backendProtocol: backendProtocol,
	}
}
