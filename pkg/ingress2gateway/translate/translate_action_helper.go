package translate

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func isUseAnnotation(portName string) bool {
	return portName == utils.ServicePortUseAnnotation
}

// actionResult holds the Gateway API resources produced from translating a single action annotation.
type actionResult struct {
	// BackendRefs for the HTTPRoute rule (forward actions).
	BackendRefs []gwv1.HTTPBackendRef
	// Filters for the HTTPRoute rule (redirect actions).
	Filters []gwv1.HTTPRouteFilter
	// ListenerRuleConfiguration to emit when the action requires one (fixed-response, stickiness, redirect query).
	ListenerRuleConfiguration *gatewayv1beta1.ListenerRuleConfiguration
	// ServiceRefs tracks K8s services referenced by forward actions (for TGC generation).
	ServiceRefs []serviceRef
}

func translateAction(action *ingress.Action, namespace, svcName string, servicesByKey map[string]corev1.Service) (*actionResult, error) {
	switch action.Type {
	case ingress.ActionTypeForward:
		return translateForwardAction(action, namespace, svcName, servicesByKey)
	case ingress.ActionTypeRedirect:
		return translateRedirectAction(action, namespace, svcName)
	case ingress.ActionTypeFixedResponse:
		return translateFixedResponseAction(action, namespace, svcName)
	default:
		return nil, fmt.Errorf("unsupported action type %q", action.Type)
	}
}

func translateForwardAction(action *ingress.Action, namespace, svcName string, servicesByKey map[string]corev1.Service) (*actionResult, error) {
	if action.ForwardConfig == nil {
		return nil, fmt.Errorf("forward action %q missing forwardConfig", svcName)
	}
	result := &actionResult{}
	for _, tg := range action.ForwardConfig.TargetGroups {
		ref, err := buildBackendRefFromTG(tg, namespace, servicesByKey)
		if err != nil {
			return nil, fmt.Errorf("forward action %q: %w", svcName, err)
		}
		result.BackendRefs = append(result.BackendRefs, ref)
		if tg.ServiceName != nil && ref.Port != nil {
			result.ServiceRefs = append(result.ServiceRefs, serviceRef{
				namespace: namespace,
				name:      *tg.ServiceName,
				port:      int32(*ref.Port),
			})
		}
	}

	// If stickiness is configured, emit a ListenerRuleConfiguration with forwardConfig.
	if sc := action.ForwardConfig.TargetGroupStickinessConfig; sc != nil {
		lrc := buildListenerRuleConfiguration(namespace, svcName)
		lrc.Spec.Actions = []gatewayv1beta1.Action{
			{
				Type: gatewayv1beta1.ActionTypeForward,
				ForwardConfig: &gatewayv1beta1.ForwardActionConfig{
					TargetGroupStickinessConfig: &gatewayv1beta1.TargetGroupStickinessConfig{
						Enabled:         sc.Enabled,
						DurationSeconds: sc.DurationSeconds,
					},
				},
			},
		}
		result.ListenerRuleConfiguration = lrc
		result.Filters = append(result.Filters, extensionRefFilter(lrc.Name))
	}
	return result, nil
}

func translateRedirectAction(a *ingress.Action, namespace, svcName string) (*actionResult, error) {
	if a.RedirectConfig == nil {
		return nil, fmt.Errorf("redirect action %q missing redirectConfig", svcName)
	}
	rc := a.RedirectConfig
	result := &actionResult{}

	redirect := gwv1.HTTPRequestRedirectFilter{}
	if rc.Host != nil {
		hostname := gwv1.PreciseHostname(*rc.Host)
		redirect.Hostname = &hostname
	}
	if rc.Path != nil {
		redirect.Path = &gwv1.HTTPPathModifier{
			Type:            gwv1.FullPathHTTPPathModifier,
			ReplaceFullPath: rc.Path,
		}
	}
	if rc.Port != nil {
		portNum, err := strconv.ParseInt(*rc.Port, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("redirect action %q: invalid port %q: %w", svcName, *rc.Port, err)
		}
		port := gwv1.PortNumber(portNum)
		redirect.Port = &port
	}
	if rc.Protocol != nil {
		scheme := strings.ToLower(*rc.Protocol)
		redirect.Scheme = &scheme
	}
	if rc.StatusCode != "" {
		code, err := redirectStatusCode(rc.StatusCode)
		if err != nil {
			return nil, fmt.Errorf("redirect action %q: %w", svcName, err)
		}
		redirect.StatusCode = &code
	}
	result.Filters = append(result.Filters, gwv1.HTTPRouteFilter{
		Type:            gwv1.HTTPRouteFilterRequestRedirect,
		RequestRedirect: &redirect,
	})

	// If query is set and not the default passthrough, emit LRC with redirectConfig.query.
	if rc.Query != nil && *rc.Query != "#{query}" {
		lrc := buildListenerRuleConfiguration(namespace, svcName)
		lrc.Spec.Actions = []gatewayv1beta1.Action{
			{
				Type: gatewayv1beta1.ActionTypeRedirect,
				RedirectConfig: &gatewayv1beta1.RedirectActionConfig{
					Query: rc.Query,
				},
			},
		}
		result.ListenerRuleConfiguration = lrc
		result.Filters = append(result.Filters, extensionRefFilter(lrc.Name))
	}
	return result, nil
}

func translateFixedResponseAction(a *ingress.Action, namespace, svcName string) (*actionResult, error) {
	if a.FixedResponseConfig == nil {
		return nil, fmt.Errorf("fixed-response action %q missing fixedResponseConfig", svcName)
	}
	frc := a.FixedResponseConfig
	statusCode, err := strconv.ParseInt(frc.StatusCode, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("fixed-response action %q: invalid statusCode %q: %w", svcName, frc.StatusCode, err)
	}

	lrc := buildListenerRuleConfiguration(namespace, svcName)
	lrc.Spec.Actions = []gatewayv1beta1.Action{
		{
			Type: gatewayv1beta1.ActionTypeFixedResponse,
			FixedResponseConfig: &gatewayv1beta1.FixedResponseActionConfig{
				StatusCode:  int32(statusCode),
				ContentType: frc.ContentType,
				MessageBody: frc.MessageBody,
			},
		},
	}

	return &actionResult{
		ListenerRuleConfiguration: lrc,
		Filters:                   []gwv1.HTTPRouteFilter{extensionRefFilter(lrc.Name)},
	}, nil
}

// buildBackendRefFromTG converts a targetGroupTuple into a Gateway API HTTPBackendRef.
func buildBackendRefFromTG(tg ingress.TargetGroupTuple, namespace string, servicesByKey map[string]corev1.Service) (gwv1.HTTPBackendRef, error) {
	ref := gwv1.HTTPBackendRef{}

	switch {
	case tg.ServiceName != nil:
		port, err := resolveServicePortFromTG(tg, namespace, servicesByKey)
		if err != nil {
			return ref, err
		}
		gwPort := gwv1.PortNumber(port)
		ref.BackendRef = gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Name: gwv1.ObjectName(*tg.ServiceName),
				Port: &gwPort,
			},
			Weight: tg.Weight,
		}

	case tg.TargetGroupName != nil:
		// TODO: External TGs can only be associated with one ALB at a time. During side-by-side
		// migration, the Gateway ALB will fail to attach the same TG that the Ingress ALB uses.
		// Consider auto-duplicating external TGs during migration to enable zero-downtime cutover.
		kind := gwv1.Kind(utils.TargetGroupNameBackendKind)
		ref.BackendRef = gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Kind: &kind,
				Name: gwv1.ObjectName(*tg.TargetGroupName),
			},
			Weight: tg.Weight,
		}

	case tg.TargetGroupARN != nil:
		// External target group by ARN — extract the TG name from the ARN.
		// The gateway controller only supports TargetGroupName kind, not ARN directly,
		// so we extract the name component. TG names are unique within a region+account,
		// which is the scope of a single LBC deployment.
		// TODO: Same external TG limitation as TargetGroupName above.
		tgName := extractTGNameFromARN(*tg.TargetGroupARN)
		kind := gwv1.Kind(utils.TargetGroupNameBackendKind)
		ref.BackendRef = gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Kind: &kind,
				Name: gwv1.ObjectName(tgName),
			},
			Weight: tg.Weight,
		}

	default:
		return ref, fmt.Errorf("targetGroupTuple has no serviceName, targetGroupName, or targetGroupARN")
	}

	return ref, nil
}

// resolveServicePortFromTG resolves the numeric port from a targetGroupTuple's servicePort.
func resolveServicePortFromTG(tg ingress.TargetGroupTuple, namespace string, servicesByKey map[string]corev1.Service) (int32, error) {
	if tg.ServicePort == nil {
		return 0, fmt.Errorf("service %q missing servicePort", *tg.ServiceName)
	}
	if tg.ServicePort.Type == intstr.Int {
		return int32(tg.ServicePort.IntValue()), nil
	}
	// Try parsing as numeric string first (e.g., "80")
	portNum, err := strconv.ParseInt(tg.ServicePort.String(), 10, 32)
	if err == nil {
		return int32(portNum), nil
	}
	// Named port — resolve via Service object
	return lookupNamedPort(namespace, *tg.ServiceName, tg.ServicePort.String(), servicesByKey)
}

// extractTGNameFromARN extracts the target group name from an ARN.
// ARN format: arn:aws:elasticloadbalancing:<region>:<account>:targetgroup/<name>/<id>
func extractTGNameFromARN(arn string) string {
	idx := strings.Index(arn, utils.TargetGroupARNPrefix)
	if idx == -1 {
		return arn
	}
	rest := arn[idx+len(utils.TargetGroupARNPrefix):]
	if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
		return rest[:slashIdx]
	}
	return rest
}

// redirectStatusCode converts ALB redirect status codes (HTTP_301, HTTP_302) to int.
func redirectStatusCode(code string) (int, error) {
	switch code {
	case "HTTP_301":
		return 301, nil
	case "HTTP_302":
		return 302, nil
	default:
		if n, err := strconv.Atoi(code); err == nil {
			return n, nil
		}
		return 0, fmt.Errorf("unsupported redirect status code %q", code)
	}
}
