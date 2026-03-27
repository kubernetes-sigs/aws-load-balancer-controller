package translate

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	sharedconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// buildHTTPRoutes builds one or more HTTPRoutes from an Ingress resource and collects
// unique service references (with resolved ports) encountered during the iteration.
// When an Ingress has both a defaultBackend and host-based rules, the default backend
// is emitted as a separate HTTPRoute without hostnames so it becomes a true catch-all,
// matching the Ingress behavior where the default backend handles any request regardless
// of hostname.
// It also handles "use-annotation" backends by parsing the corresponding actions.* annotation
// and translating it into the appropriate Gateway API constructs (backendRefs, filters, ListenerRuleConfigs).
func buildHTTPRoutes(ing networking.Ingress, namespace, gatewayName string, listenPorts []listenPortEntry, servicesByKey map[string]corev1.Service) ([]gwv1.HTTPRoute, []serviceRef, []gatewayv1beta1.ListenerRuleConfiguration, error) {
	useRegex := getString(ing.Annotations, annotations.IngressSuffixUseRegexPathMatch) == "true"
	// Build parentRefs — one per listener on the gateway
	var parentRefs []gwv1.ParentReference

	for _, lp := range listenPorts {
		sectionName := gwv1.SectionName(utils.GetSectionName(lp.Protocol, lp.Port))
		parentRefs = append(parentRefs, gwv1.ParentReference{
			Name:        gwv1.ObjectName(gatewayName),
			SectionName: &sectionName,
		})
	}

	// Collect unique service refs during iteration
	svcRefSeen := sets.New[string]()
	var svcRefs []serviceRef
	trackBackend := func(svcName string, resolvedPort int32) {
		ref := serviceRef{namespace: namespace, name: svcName, port: resolvedPort}
		if !svcRefSeen.Has(ref.getServiceRefKey()) {
			svcRefSeen.Insert(ref.getServiceRefKey())
			svcRefs = append(svcRefs, ref)
		}
	}

	// Build rules from Ingress spec.rules
	var rules []gwv1.HTTPRouteRule
	var hostnames []gwv1.Hostname

	// Collect LRCs produced by use-annotation backends
	var listenerRuleConfigs []gatewayv1beta1.ListenerRuleConfiguration

	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hostnames = append(hostnames, gwv1.Hostname(rule.Host))
		}
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			routeRule := gwv1.HTTPRouteRule{}

			// Build match
			match := gwv1.HTTPRouteMatch{}
			if path.Path != "" {
				pathType, err := toGatewayPathType(path.PathType, useRegex)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("ingress %s/%s path %q: %w", namespace, ing.Name, path.Path, err)
				}
				pathValue := path.Path
				// When using regex path match with ImplementationSpecific, the Ingress controller
				// strips the leading "/" from the path (it's a K8s API requirement, not part of the regex).
				// Gateway API takes the regex as-is, so we strip it here to preserve the same behavior.
				if useRegex && path.PathType != nil && *path.PathType == networking.PathTypeImplementationSpecific && len(pathValue) > 1 && pathValue[0] == '/' {
					pathValue = pathValue[1:]
				}
				match.Path = &gwv1.HTTPPathMatch{
					Type:  &pathType,
					Value: &pathValue,
				}
			}
			routeRule.Matches = []gwv1.HTTPRouteMatch{match}

			// Build backendRef or translate use-annotation action
			if path.Backend.Service != nil {
				if isUseAnnotation(path.Backend.Service.Port.Name) {
					// "use-annotation" backend: parse the actions.* annotation and translate it.
					svcName := path.Backend.Service.Name
					parsedAction, err := parseActionAnnotation(ing.Annotations, svcName)
					if err != nil {
						return nil, nil, nil, fmt.Errorf("ingress %s/%s failed in parse action annotation: %w", namespace, ing.Name, err)
					}
					actionResult, err := translateAction(parsedAction, namespace, svcName, servicesByKey)
					if err != nil {
						return nil, nil, nil, fmt.Errorf("ingress %s/%s failed in translate action %q: %w", namespace, ing.Name, svcName, err)
					}
					if len(actionResult.BackendRefs) > 0 {
						routeRule.BackendRefs = actionResult.BackendRefs
					}
					if len(actionResult.Filters) > 0 {
						routeRule.Filters = actionResult.Filters
					}
					if actionResult.ListenerRuleConfiguration != nil {
						listenerRuleConfigs = append(listenerRuleConfigs, *actionResult.ListenerRuleConfiguration)
					}
					// Track K8s service backends from forward actions for TGC generation
					for _, ref := range actionResult.ServiceRefs {
						trackBackend(ref.name, ref.port)
					}
				} else {
					portNum, err := resolveServicePort(path.Backend.Service.Port, namespace, path.Backend.Service.Name, servicesByKey)
					if err != nil {
						return nil, nil, nil, err
					}
					trackBackend(path.Backend.Service.Name, portNum)
					port := gwv1.PortNumber(portNum)
					routeRule.BackendRefs = []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(path.Backend.Service.Name),
									Port: &port,
								},
							},
						},
					}
				}
			}

			rules = append(rules, routeRule)
		}
	}

	// get hostnames from spec.rules and spec.tls
	for _, tls := range ing.Spec.TLS {
		for _, h := range tls.Hosts {
			hostnames = append(hostnames, gwv1.Hostname(h))
		}
	}
	hostnames = deduplicateHostnames(hostnames)

	// Build default backend rule if present
	var defaultRule *gwv1.HTTPRouteRule
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		portNum, err := resolveServicePort(ing.Spec.DefaultBackend.Service.Port, namespace, ing.Spec.DefaultBackend.Service.Name, servicesByKey)
		if err != nil {
			return nil, nil, nil, err
		}
		trackBackend(ing.Spec.DefaultBackend.Service.Name, portNum)
		port := gwv1.PortNumber(portNum)
		defaultRule = &gwv1.HTTPRouteRule{
			BackendRefs: []gwv1.HTTPBackendRef{
				{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: gwv1.ObjectName(ing.Spec.DefaultBackend.Service.Name),
							Port: &port,
						},
					},
				},
			},
		}
	}

	// When no hostnames, the default backend can live in the same route.
	// When hostnames are present, it needs a separate route to be a true catch-all.
	hasHostnames := len(hostnames) > 0
	if defaultRule != nil && !hasHostnames {
		rules = append(rules, *defaultRule)
		defaultRule = nil
	}

	var routes []gwv1.HTTPRoute
	// Primary route (path rules + possibly the default rule when no hostnames)
	routes = append(routes, newHTTPRoute(utils.GetHTTPRouteName(namespace, ing.Name), namespace, parentRefs, hostnames, rules))

	// Separate catch-all route for defaultBackend when hostnames are present
	if defaultRule != nil {
		routes = append(routes, newHTTPRoute(utils.GetDefaultHTTPRouteName(namespace, ing.Name), namespace, parentRefs, nil, []gwv1.HTTPRouteRule{*defaultRule}))
	}

	return routes, svcRefs, listenerRuleConfigs, nil
}

// resolveServicePort resolves a ServiceBackendPort to a numeric port.
// If the port is specified by number, it's returned directly.
// If specified by name, the Service is looked up to find the matching port number.
func resolveServicePort(sbp networking.ServiceBackendPort, namespace, svcName string, servicesByKey map[string]corev1.Service) (int32, error) {
	if sbp.Number != 0 {
		return sbp.Number, nil
	}
	if sbp.Name == "" {
		return 0, fmt.Errorf("service %s/%s has no port number or name", namespace, svcName)
	}
	return lookupNamedPort(namespace, svcName, sbp.Name, servicesByKey)
}

// lookupNamedPort resolves a named port to a numeric port by looking up the Service object.
func lookupNamedPort(namespace, svcName, portName string, servicesByKey map[string]corev1.Service) (int32, error) {
	svcKey := fmt.Sprintf("%s/%s", namespace, svcName)
	svc, ok := servicesByKey[svcKey]
	if !ok {
		return 0, fmt.Errorf("service %s not found, cannot resolve named port %q", svcKey, portName)
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == portName {
			return p.Port, nil
		}
	}
	return 0, fmt.Errorf("service %s has no port named %q", svcKey, portName)
}

// newHTTPRoute constructs an HTTPRoute with the given parameters.
func newHTTPRoute(name, namespace string, parentRefs []gwv1.ParentReference, hostnames []gwv1.Hostname, rules []gwv1.HTTPRouteRule) gwv1.HTTPRoute {
	return gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwconstants.ALBRouteResourceGroupVersion,
			Kind:       sharedconstants.HTTPRouteKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Hostnames: hostnames,
			Rules:     rules,
		},
	}
}

// toGatewayPathType converts Ingress pathType to Gateway API path match type.
// When useRegex is true and pathType is ImplementationSpecific, it maps to RegularExpression.
func toGatewayPathType(pt *networking.PathType, useRegex bool) (gwv1.PathMatchType, error) {
	if pt == nil {
		return gwv1.PathMatchPathPrefix, nil
	}
	switch *pt {
	case networking.PathTypeExact:
		return gwv1.PathMatchExact, nil
	case networking.PathTypePrefix:
		return gwv1.PathMatchPathPrefix, nil
	case networking.PathTypeImplementationSpecific:
		if useRegex {
			return gwv1.PathMatchRegularExpression, nil
		}
		return gwv1.PathMatchPathPrefix, nil
	default:
		return "", fmt.Errorf("unsupported path type: %v", *pt)
	}
}

func deduplicateHostnames(hostnames []gwv1.Hostname) []gwv1.Hostname {
	seen := sets.New[string]()
	var result []gwv1.Hostname
	for _, h := range hostnames {
		lower := strings.ToLower(string(h))
		if !seen.Has(lower) {
			seen.Insert(lower)
			result = append(result, h)
		}
	}
	return result
}
