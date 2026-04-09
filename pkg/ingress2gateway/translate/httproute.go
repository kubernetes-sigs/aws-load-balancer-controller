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

// hostScopedRoute holds a rule that was separated into its own HTTPRoute because it has
// a host-header condition with values that differ from the Ingress spec host.
// Gateway API hostnames are route-level, so per-rule host overrides require separate routes.
type hostScopedRoute struct {
	svcName   string
	rule      gwv1.HTTPRouteRule
	hostnames []gwv1.Hostname
}

// httpRouteTranslator holds shared state accumulated while iterating over Ingress paths.
type httpRouteTranslator struct {
	namespace           string
	ingName             string
	ingAnnotations      map[string]string
	useRegex            bool
	servicesByKey       map[string]corev1.Service
	svcRefSeen          sets.Set[string]
	svcRefs             []serviceRef
	listenerRuleConfigs []gatewayv1beta1.ListenerRuleConfiguration
}

// trackBackend records a unique service reference for TGC generation.
func (t *httpRouteTranslator) trackBackend(svcName string, resolvedPort int32) {
	ref := serviceRef{namespace: t.namespace, name: svcName, port: resolvedPort}
	if !t.svcRefSeen.Has(ref.getServiceRefKey()) {
		t.svcRefSeen.Insert(ref.getServiceRefKey())
		t.svcRefs = append(t.svcRefs, ref)
	}
}

// buildHTTPRoutes builds one or more HTTPRoutes from an Ingress resource and collects
func buildHTTPRoutes(ing networking.Ingress, namespace, gatewayName string, listenPorts []listenPortEntry, servicesByKey map[string]corev1.Service, sslRedirectPort *int32) ([]gwv1.HTTPRoute, []serviceRef, []gatewayv1beta1.ListenerRuleConfiguration, error) {
	// Determine parentRefs based on ssl-redirect.
	// When ssl-redirect is set, the rules route attaches only to the HTTPS listener.
	// When not set, the route attaches to all listeners (no sectionName).
	var parentRefs []gwv1.ParentReference
	if sslRedirectPort != nil {
		sectionName := utils.GenerateSectionName(utils.ProtocolHTTPS, *sslRedirectPort)
		parentRefs = buildParentRefs(gatewayName, &sectionName)
	} else {
		parentRefs = buildParentRefs(gatewayName, nil)
	}

	t := &httpRouteTranslator{
		namespace:      namespace,
		ingName:        ing.Name,
		ingAnnotations: ing.Annotations,
		useRegex:       strings.EqualFold(getString(ing.Annotations, annotations.IngressSuffixUseRegexPathMatch), "true"),
		servicesByKey:  servicesByKey,
		svcRefSeen:     sets.New[string](),
	}

	var rules []gwv1.HTTPRouteRule
	var hostnames []gwv1.Hostname
	var hostScopedRoutes []hostScopedRoute

	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hostnames = append(hostnames, gwv1.Hostname(rule.Host))
		}
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			routeRule, hsr, err := t.buildRouteRule(rule, path)
			if err != nil {
				return nil, nil, nil, err
			}
			if hsr != nil {
				hostScopedRoutes = append(hostScopedRoutes, *hsr)
			} else {
				rules = append(rules, routeRule)
			}
		}
	}

	// Collect hostnames from spec.tls
	for _, tls := range ing.Spec.TLS {
		for _, h := range tls.Hosts {
			hostnames = append(hostnames, gwv1.Hostname(h))
		}
	}
	hostnames = deduplicateHostnames(hostnames)

	routes, err := assembleRoutes(namespace, ing.Name, parentRefs, hostnames, rules, hostScopedRoutes,
		ing.Spec.DefaultBackend, t)
	if err != nil {
		return nil, nil, nil, err
	}

	// When ssl-redirect is set, generate a redirect route for each HTTP listener.
	if sslRedirectPort != nil {
		for _, lp := range listenPorts {
			if strings.EqualFold(lp.Protocol, utils.ProtocolHTTP) {
				redirectRoute := buildSSLRedirectRoute(
					namespace, ing.Name, gatewayName,
					utils.GenerateSectionName(lp.Protocol, lp.Port),
					*sslRedirectPort,
				)
				routes = append(routes, redirectRoute)
			}
		}
	}

	return routes, t.svcRefs, t.listenerRuleConfigs, nil
}

// buildParentRefs creates a ParentReference to the gateway.
// When sectionName is nil, the route attaches to all listeners.
// When sectionName is set, the route attaches only to that specific listener.
func buildParentRefs(gatewayName string, sectionName *string) []gwv1.ParentReference {
	ref := gwv1.ParentReference{
		Name: gwv1.ObjectName(gatewayName),
	}
	if sectionName != nil {
		sn := gwv1.SectionName(*sectionName)
		ref.SectionName = &sn
	}
	return []gwv1.ParentReference{ref}
}

// buildRouteRule builds a single HTTPRouteRule from an Ingress path, including backend resolution,
// action annotation translation, and condition annotation translation.
func (t *httpRouteTranslator) buildRouteRule(rule networking.IngressRule, path networking.HTTPIngressPath) (gwv1.HTTPRouteRule, *hostScopedRoute, error) {
	routeRule := gwv1.HTTPRouteRule{}

	match, err := buildPathMatch(path, t.useRegex)
	if err != nil {
		return routeRule, nil, fmt.Errorf("ingress %s/%s path %q: %w", t.namespace, t.ingName, path.Path, err)
	}
	routeRule.Matches = []gwv1.HTTPRouteMatch{match}

	if path.Backend.Service != nil {
		// pre-routing actions
		if err := t.buildPreRoutingActions(&routeRule, path.Backend.Service.Name); err != nil {
			return routeRule, nil, err
		}
		// routing actions
		if err := t.buildBackendForRule(&routeRule, path.Backend.Service); err != nil {
			return routeRule, nil, err
		}
		hsr, err := t.buildConditions(&routeRule, rule, path.Backend.Service.Name)
		if err != nil {
			return routeRule, nil, err
		}
		if err := t.buildTransforms(&routeRule, path.Backend.Service.Name); err != nil {
			return routeRule, nil, err
		}
		if hsr != nil {
			return routeRule, hsr, nil
		}
	}

	return routeRule, nil, nil
}

// buildPathMatch builds an HTTPRouteMatch from an Ingress path spec.
func buildPathMatch(path networking.HTTPIngressPath, useRegex bool) (gwv1.HTTPRouteMatch, error) {
	match := gwv1.HTTPRouteMatch{}
	if path.Path != "" {
		pathType, err := toGatewayPathType(path.PathType, useRegex)
		if err != nil {
			return match, err
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
	return match, nil
}

// buildBackendForRule resolves the backend for a route rule — either a use-annotation action
// or a real K8s service reference.
func (t *httpRouteTranslator) buildBackendForRule(routeRule *gwv1.HTTPRouteRule, svcBackend *networking.IngressServiceBackend) error {
	if isUseAnnotation(svcBackend.Port.Name) {
		return t.buildUseAnnotationBackend(routeRule, svcBackend.Name)
	}
	return t.buildServiceBackend(routeRule, svcBackend)
}

func (t *httpRouteTranslator) buildUseAnnotationBackend(routeRule *gwv1.HTTPRouteRule, svcName string) error {
	parsedAction, err := parseActionAnnotation(t.ingAnnotations, svcName)
	if err != nil {
		return fmt.Errorf("ingress %s/%s failed in parse action annotation: %w", t.namespace, t.ingName, err)
	}
	actionResult, err := translateAction(parsedAction, t.namespace, t.ingName, svcName, t.servicesByKey)
	if err != nil {
		return fmt.Errorf("ingress %s/%s failed in translate action %q: %w", t.namespace, t.ingName, svcName, err)
	}
	if len(actionResult.BackendRefs) > 0 {
		routeRule.BackendRefs = actionResult.BackendRefs
	}
	if len(actionResult.Filters) > 0 {
		routeRule.Filters = actionResult.Filters
	}
	if actionResult.ListenerRuleConfiguration != nil {
		t.listenerRuleConfigs = append(t.listenerRuleConfigs, *actionResult.ListenerRuleConfiguration)
	}
	for _, ref := range actionResult.ServiceRefs {
		t.trackBackend(ref.name, ref.port)
	}
	return nil
}

func (t *httpRouteTranslator) buildServiceBackend(routeRule *gwv1.HTTPRouteRule, svcBackend *networking.IngressServiceBackend) error {
	portNum, err := resolveServicePort(svcBackend.Port, t.namespace, svcBackend.Name, t.servicesByKey)
	if err != nil {
		return err
	}
	t.trackBackend(svcBackend.Name, portNum)
	port := gwv1.PortNumber(portNum)
	routeRule.BackendRefs = []gwv1.HTTPBackendRef{
		{
			BackendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name: gwv1.ObjectName(svcBackend.Name),
					Port: &port,
				},
			},
		},
	}
	return nil
}

// buildConditions parses and applies the conditions.* annotation for a rule.
// Returns a hostScopedRoute if the condition has host-header values (requiring a separate HTTPRoute),
// or nil if the rule stays in the primary route.
func (t *httpRouteTranslator) buildConditions(routeRule *gwv1.HTTPRouteRule, ingressRule networking.IngressRule, svcName string) (*hostScopedRoute, error) {
	parsedConditions, err := parseConditionAnnotation(t.ingAnnotations, svcName)
	if err != nil {
		return nil, fmt.Errorf("ingress %s/%s failed to parse condition annotation for %q: %w", t.namespace, t.ingName, svcName, err)
	}
	if len(parsedConditions) == 0 {
		return nil, nil
	}

	condResult := translateConditions(parsedConditions, routeRule.Matches)
	if condResult == nil {
		return nil, nil
	}

	routeRule.Matches = condResult.Matches

	if len(condResult.ListenerRuleConditions) > 0 {
		lrc := findOrCreateLRC(&t.listenerRuleConfigs, t.namespace, t.ingName, svcName)
		lrc.Spec.Conditions = append(lrc.Spec.Conditions, condResult.ListenerRuleConditions...)
		if !routeRuleHasExtensionRef(*routeRule, lrc.Name) {
			routeRule.Filters = append(routeRule.Filters, extensionRefFilter(lrc.Name))
		}
	}

	// Host-header values require a separate HTTPRoute because hostnames are route-level.
	if len(condResult.AdditionalHostnames) > 0 {
		var ruleHostnames []gwv1.Hostname
		if ingressRule.Host != "" {
			ruleHostnames = append(ruleHostnames, gwv1.Hostname(ingressRule.Host))
		}
		ruleHostnames = append(ruleHostnames, condResult.AdditionalHostnames...)
		ruleHostnames = deduplicateHostnames(ruleHostnames)
		return &hostScopedRoute{
			svcName:   svcName,
			rule:      *routeRule,
			hostnames: ruleHostnames,
		}, nil
	}

	return nil, nil
}

// buildTransforms parses and applies the transforms.* annotation for a rule.
func (t *httpRouteTranslator) buildTransforms(routeRule *gwv1.HTTPRouteRule, svcName string) error {
	parsedTransforms, err := parseTransformAnnotation(t.ingAnnotations, svcName)
	if err != nil {
		return fmt.Errorf("ingress %s/%s failed to parse transform annotation for %q: %w", t.namespace, t.ingName, svcName, err)
	}
	if len(parsedTransforms) == 0 {
		return nil
	}

	filter := translateTransforms(parsedTransforms)
	if filter != nil {
		routeRule.Filters = append(routeRule.Filters, *filter)
	}
	return nil
}

// buildPreRoutingActions parses auth cognito/oidc and jwt-validation annotations
// and attaches pre-routing actions to the ListenerRuleConfiguration for this rule.
// Auth and JWT validation are mutually exclusive pre-routing actions.
func (t *httpRouteTranslator) buildPreRoutingActions(routeRule *gwv1.HTTPRouteRule, svcName string) error {
	authAction, err := buildAuthAction(t.ingAnnotations)
	if err != nil {
		return fmt.Errorf("ingress %s/%s failed to build auth config: %w", t.namespace, t.ingName, err)
	}

	jwtAction, err := buildJwtValidationAction(t.ingAnnotations)
	if err != nil {
		return fmt.Errorf("ingress %s/%s failed to build jwt-validation config: %w", t.namespace, t.ingName, err)
	}

	if authAction != nil && jwtAction != nil {
		return fmt.Errorf("ingress %s/%s has both auth-type and jwt-validation annotations; only one pre-routing action is allowed", t.namespace, t.ingName)
	}

	action := authAction
	if action == nil {
		action = jwtAction
	}
	if action == nil {
		return nil
	}

	lrc := findOrCreateLRC(&t.listenerRuleConfigs, t.namespace, t.ingName, svcName)
	// Pre-routing actions are per-Ingress (not per-rule), so skip if the LRC
	// already has one from a previous rule sharing the same LRC.
	if !lrcHasPreRoutingAction(lrc) {
		lrc.Spec.Actions = append(lrc.Spec.Actions, *action)
	}
	if !routeRuleHasExtensionRef(*routeRule, lrc.Name) {
		routeRule.Filters = append(routeRule.Filters, extensionRefFilter(lrc.Name))
	}
	return nil
}

// assembleRoutes builds the final list of HTTPRoutes from the primary rules and default backend.
func assembleRoutes(namespace, ingName string, parentRefs []gwv1.ParentReference, hostnames []gwv1.Hostname,
	rules []gwv1.HTTPRouteRule, hostScopedRoutes []hostScopedRoute,
	defaultBackend *networking.IngressBackend, t *httpRouteTranslator) ([]gwv1.HTTPRoute, error) {

	// Build default backend rule if present
	var defaultRule *gwv1.HTTPRouteRule
	if defaultBackend != nil && defaultBackend.Service != nil {
		portNum, err := resolveServicePort(defaultBackend.Service.Port, namespace, defaultBackend.Service.Name, t.servicesByKey)
		if err != nil {
			return nil, fmt.Errorf("ingress %s/%s defaultBackend: %w", namespace, ingName, err)
		}
		t.trackBackend(defaultBackend.Service.Name, portNum)
		port := gwv1.PortNumber(portNum)
		defaultRule = &gwv1.HTTPRouteRule{
			BackendRefs: []gwv1.HTTPBackendRef{
				{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: gwv1.ObjectName(defaultBackend.Service.Name),
							Port: &port,
						},
					},
				},
			},
		}
	}

	// When no hostnames, the default backend can live in the same route.
	// When hostnames are present, it needs a separate route to be a true catch-all.
	if defaultRule != nil && len(hostnames) == 0 {
		rules = append(rules, *defaultRule)
		defaultRule = nil
	}

	var routes []gwv1.HTTPRoute

	// Primary route
	if len(rules) > 0 || defaultRule == nil {
		routes = append(routes, newHTTPRoute(utils.GetHTTPRouteName(namespace, ingName), namespace, parentRefs, hostnames, rules))
	}

	// Separate catch-all route for defaultBackend when hostnames are present
	if defaultRule != nil {
		routes = append(routes, newHTTPRoute(utils.GetDefaultHTTPRouteName(namespace, ingName), namespace, parentRefs, nil, []gwv1.HTTPRouteRule{*defaultRule}))
	}

	// Separate routes for rules with host-header conditions
	for _, hsr := range hostScopedRoutes {
		routes = append(routes, newHTTPRoute(
			utils.GetHTTPRouteName(namespace, hsr.svcName),
			namespace, parentRefs, hsr.hostnames,
			[]gwv1.HTTPRouteRule{hsr.rule},
		))
	}

	return routes, nil
}

// buildSSLRedirectRoute creates an HTTPRoute that redirects all HTTP traffic to HTTPS.
// It attaches only to the specified HTTP listener via sectionName.
func buildSSLRedirectRoute(namespace, ingName, gatewayName, httpSectionName string, sslPort int32) gwv1.HTTPRoute {
	parentRefs := buildParentRefs(gatewayName, &httpSectionName)
	httpsScheme := "https"
	port := gwv1.PortNumber(sslPort)
	statusCode := 301
	return newHTTPRoute(
		utils.GetRedirectHTTPRouteName(namespace, ingName),
		namespace, parentRefs, nil,
		[]gwv1.HTTPRouteRule{{
			Filters: []gwv1.HTTPRouteFilter{{
				Type: gwv1.HTTPRouteFilterRequestRedirect,
				RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
					Scheme:     &httpsScheme,
					Port:       &port,
					StatusCode: &statusCode,
				},
			}},
		}},
	)
}

// resolveServicePort resolves a ServiceBackendPort to a numeric port.
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
