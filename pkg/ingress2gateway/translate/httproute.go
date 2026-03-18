package translate

import (
	"strings"

	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	sharedconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// buildHTTPRoutes builds one or more HTTPRoutes from an Ingress resource.
// When an Ingress has both a defaultBackend and host-based rules, the default backend
// is emitted as a separate HTTPRoute without hostnames so it becomes a true catch-all,
// matching the Ingress behavior where the default backend handles any request regardless
// of hostname.
func buildHTTPRoutes(ing networking.Ingress, namespace, gatewayName string, listenPorts []listenPortEntry) []gwv1.HTTPRoute {
	useRegex := ing.Annotations[annotationKey(annotations.IngressSuffixUseRegexPathMatch)] == "true"
	// Build parentRefs — one per listener on the gateway
	var parentRefs []gwv1.ParentReference

	for _, lp := range listenPorts {
		sectionName := gwv1.SectionName(utils.GetListenerName(lp.Protocol, lp.Port))
		parentRefs = append(parentRefs, gwv1.ParentReference{
			Name:        gwv1.ObjectName(gatewayName),
			SectionName: &sectionName,
		})
	}

	// Build rules from Ingress spec.rules
	var rules []gwv1.HTTPRouteRule
	var hostnames []gwv1.Hostname

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
				pathType := toGatewayPathType(path.PathType, useRegex)
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

			// Build backendRef
			if path.Backend.Service != nil {
				port := gwv1.PortNumber(path.Backend.Service.Port.Number)
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
		port := gwv1.PortNumber(ing.Spec.DefaultBackend.Service.Port.Number)
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

	return routes
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
func toGatewayPathType(pt *networking.PathType, useRegex bool) gwv1.PathMatchType {
	if pt == nil {
		return gwv1.PathMatchPathPrefix
	}
	switch *pt {
	case networking.PathTypeExact:
		return gwv1.PathMatchExact
	case networking.PathTypePrefix:
		return gwv1.PathMatchPathPrefix
	case networking.PathTypeImplementationSpecific:
		if useRegex {
			return gwv1.PathMatchRegularExpression
		}
		return gwv1.PathMatchPathPrefix
	default:
		return gwv1.PathMatchPathPrefix
	}
}

func deduplicateHostnames(hostnames []gwv1.Hostname) []gwv1.Hostname {
	seen := make(map[string]struct{})
	var result []gwv1.Hostname
	for _, h := range hostnames {
		lower := strings.ToLower(string(h))
		if _, ok := seen[lower]; !ok {
			seen[lower] = struct{}{}
			result = append(result, h)
		}
	}
	return result
}
