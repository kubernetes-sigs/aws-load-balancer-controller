package service

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

// buildDualStackIPv4ListenerRule builds the IPv4 target group and registers a priority-1 listener rule
// that forwards IPv4-sourced traffic to it.
//
// For a dual-stack NLB whose service has both IP families, buildTargetGroupIPAddressType returns
// IPv6 as the default type, so the listener's default action (set by buildDualStackDefaultActions)
// already forwards to the IPv6 TG. This rule covers the IPv4 case; no IPv6 rule is needed because
// unmatched traffic falls through to the IPv6 default action.
//
// tgProtocol must be the already-resolved protocol for this port (including any TLS/QUIC upgrades),
// matching the protocol used to build the default IPv6 target group.
//
// It is a no-op when dual-stack routing is not enabled.
func (t *defaultModelBuildTask) buildDualStackIPv4ListenerRule(ctx context.Context, ls *elbv2model.Listener, port corev1.ServicePort, cfg *listenerConfig, scheme elbv2model.LoadBalancerScheme) error {
	if !t.shouldEnableDualStackRouting() {
		return nil
	}
	tgProtocol, listenerProtocol, err := buildProtocols(port, cfg)
	if err != nil {
		return fmt.Errorf("resolving protocol for dual-stack IPv4 rule: %w", err)
	}
	switch listenerProtocol {
	case elbv2model.ProtocolTCP, elbv2model.ProtocolTLS:
		// listener rules are supported
	default:
		return errors.Errorf("dual-stack routing is not supported on %v listeners", listenerProtocol)
	}

	targetType, err := t.buildTargetType(ctx, t.service, t.service.Annotations, port)
	if err != nil {
		return fmt.Errorf("resolving target type for dual-stack routing: %w", err)
	}
	if targetType == elbv2model.TargetTypeIP {
		return errors.New("dual-stack routing is not supported with ip target type")
	}

	tgIPv4, err := t.buildTargetGroup(ctx, nil, t.service, t.service.Annotations, port, tgProtocol, scheme, elbv2model.TargetGroupIPAddressTypeIPv4)
	if err != nil {
		return fmt.Errorf("building IPv4 target group for dual-stack routing: %w", err)
	}

	ipv4Type := elbv2model.SourceIPAddressTypeIPv4
	ruleSpec := elbv2model.ListenerRuleSpec{
		ListenerARN: ls.ListenerARN(),
		Priority:    1,
		Conditions: []elbv2model.RuleCondition{
			{
				Field: elbv2model.RuleConditionFieldSourceIP,
				SourceIPConfig: &elbv2model.SourceIPConditionConfig{
					IpAddressType: &ipv4Type,
				},
			},
		},
		Actions: []elbv2model.Action{
			{
				Type: elbv2model.ActionTypeForward,
				ForwardConfig: &elbv2model.ForwardActionConfig{
					TargetGroups: []elbv2model.TargetGroupTuple{
						{TargetGroupARN: tgIPv4.TargetGroupARN()},
					},
				},
			},
		},
	}
	lrID := fmt.Sprintf("%v:ipv4-src", port.Port)
	elbv2model.NewListenerRule(t.stack, lrID, ruleSpec)
	return nil
}
