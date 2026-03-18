package translate

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	sharedconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

// Translate converts InputResources into OutputResources (Gateway API manifests).
func Translate(in *ingress2gateway.InputResources) (*ingress2gateway.OutputResources, error) {
	out := &ingress2gateway.OutputResources{}

	// Build GatewayClass
	out.GatewayClass = buildGatewayClass()

	// Build lookup maps for priority resolution
	servicesByKey := buildServiceMap(in.Services)
	ingressClassParamsByClass := buildIngressClassParamsMap(in.IngressClasses, in.IngressClassParams)

	// Track which services we've already created TGCs for (deduplicate)
	tgcCreated := make(map[string]struct{})

	for _, ing := range in.Ingresses {
		namespace := ing.Namespace
		if namespace == "" {
			namespace = "default"
		}

		// Resolve effective annotations (Ingress < Service < IngressClassParams)
		effectiveAnnotations := resolveAnnotations(ing)

		// Parse listen-ports
		listenPorts, err := parseListenPorts(effectiveAnnotations)
		if err != nil {
			return nil, fmt.Errorf("failed to parse listen ports for ingress %s/%s: %w", namespace, ing.Name, err)
		}

		// Set default listen ports
		// If no listen-ports but certificate-arn is set, default to HTTPS:443
		if len(listenPorts) == 0 {
			if getString(effectiveAnnotations, annotations.IngressSuffixCertificateARN) != "" {
				listenPorts = []listenPortEntry{{Protocol: "HTTPS", Port: 443}}
			} else {
				listenPorts = []listenPortEntry{{Protocol: "HTTP", Port: 80}}
			}
		}

		// Build LoadBalancerConfiguration (only if there are LB-level annotations)
		gatewayName := utils.GetGatewayName(namespace, ing.Name)
		lbConfigName := utils.GetLBConfigName(namespace, ing.Name)
		migrationTag := fmt.Sprintf("ingress/%s/%s", namespace, ing.Name)

		lbConfig := buildLoadBalancerConfigResource(lbConfigName, namespace, effectiveAnnotations, listenPorts, migrationTag)

		// Apply IngressClassParams overrides directly to the LB config (highest priority)
		var icp *elbv2api.IngressClassParams
		if ing.Spec.IngressClassName != nil {
			icp = ingressClassParamsByClass[*ing.Spec.IngressClassName]
		}
		if lbConfig != nil && icp != nil {
			applyIngressClassParamsToLBConfig(&lbConfig.Spec, icp)
		}

		if lbConfig != nil {
			out.LoadBalancerConfigurations = append(out.LoadBalancerConfigurations, *lbConfig)
		}

		// Build Gateway
		gw := buildGateway(gatewayName, namespace, lbConfig, listenPorts)
		out.Gateways = append(out.Gateways, gw)

		// Build HTTPRoute(s) — may produce a separate route for defaultBackend
		routes := buildHTTPRoutes(ing, namespace, gatewayName, listenPorts)
		out.HTTPRoutes = append(out.HTTPRoutes, routes...)

		// Step 7: Build TargetGroupConfigurations for each unique service
		for _, svcRef := range getServiceRefs(ing) {
			tgcKey := fmt.Sprintf("%s/%s", namespace, svcRef.name)
			if _, exists := tgcCreated[tgcKey]; exists {
				continue
			}
			tgcCreated[tgcKey] = struct{}{}

			tgAnnotations := mergeTGAnnotations(effectiveAnnotations, servicesByKey, namespace, svcRef.name)
			tgc := buildTargetGroupConfig(svcRef.name, namespace, tgAnnotations, svcRef.port, migrationTag)
			if tgc != nil {
				if icp != nil {
					applyIngressClassParamsToTGProps(&tgc.Spec.DefaultConfiguration, icp)
				}
				out.TargetGroupConfigurations = append(out.TargetGroupConfigurations, *tgc)
			}
		}
	}

	return out, nil
}

// resolveAnnotations returns the Ingress annotations as the effective annotation map.
// IngressClassParams overrides are applied directly to the output CRD structs, not via annotations.
func resolveAnnotations(ing networking.Ingress) map[string]string {
	effective := make(map[string]string)
	for k, v := range ing.Annotations {
		effective[k] = v
	}
	return effective
}

// mergeTGAnnotations merges service-level annotations for TG-specific fields.
// Service annotations take priority over Ingress annotations for per-backend fields.
func mergeTGAnnotations(ingressAnnotations map[string]string, servicesByKey map[string]corev1.Service, namespace, svcName string) map[string]string {
	merged := make(map[string]string)
	for k, v := range ingressAnnotations {
		merged[k] = v
	}

	svcKey := fmt.Sprintf("%s/%s", namespace, svcName)
	if svc, ok := servicesByKey[svcKey]; ok {
		// Service annotations override for TG-level fields
		tgSuffixes := []string{
			annotations.IngressSuffixTargetType,
			annotations.IngressSuffixBackendProtocol,
			annotations.IngressSuffixBackendProtocolVersion,
			annotations.IngressSuffixTargetGroupAttributes,
			annotations.IngressSuffixHealthCheckPort,
			annotations.IngressSuffixHealthCheckProtocol,
			annotations.IngressSuffixHealthCheckPath,
			annotations.IngressSuffixHealthCheckIntervalSeconds,
			annotations.IngressSuffixHealthCheckTimeoutSeconds,
			annotations.IngressSuffixHealthyThresholdCount,
			annotations.IngressSuffixUnhealthyThresholdCount,
			annotations.IngressSuffixSuccessCodes,
			annotations.IngressSuffixTargetNodeLabels,
			annotations.IngressLBSuffixMultiClusterTargetGroup,
			annotations.IngressSuffixTags,
		}
		for _, suffix := range tgSuffixes {
			key := annotationKey(suffix)
			if v, ok := svc.Annotations[key]; ok {
				merged[key] = v
			}
		}
	}

	return merged
}

// serviceRef holds a service name and port referenced by an Ingress backend.
type serviceRef struct {
	name string
	port int32
}

// getServiceRefs extracts all unique service references from an Ingress.
func getServiceRefs(ing networking.Ingress) []serviceRef {
	seen := make(map[string]struct{})
	var refs []serviceRef

	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		name := ing.Spec.DefaultBackend.Service.Name
		port := ing.Spec.DefaultBackend.Service.Port.Number
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			refs = append(refs, serviceRef{name: name, port: port})
		}
	}

	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			name := path.Backend.Service.Name
			port := path.Backend.Service.Port.Number
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				refs = append(refs, serviceRef{name: name, port: port})
			}
		}
	}
	return refs
}

// buildServiceMap builds a lookup map: namespace/name → Service.
func buildServiceMap(services []corev1.Service) map[string]corev1.Service {
	serviceMap := make(map[string]corev1.Service, len(services))
	for _, svc := range services {
		ns := svc.Namespace
		if ns == "" {
			ns = "default"
		}
		serviceMap[fmt.Sprintf("%s/%s", ns, svc.Name)] = svc
	}
	return serviceMap
}

// buildIngressClassParamsMap builds a lookup: IngressClass name → IngressClassParams.
func buildIngressClassParamsMap(classes []networking.IngressClass, params []elbv2api.IngressClassParams) map[string]*elbv2api.IngressClassParams {
	// map IngressClassParams by name
	paramsByName := make(map[string]*elbv2api.IngressClassParams, len(params))
	for i := range params {
		paramsByName[params[i].Name] = &params[i]
	}

	// map IngressClass name → IngressClassParams via the parametersRef
	result := make(map[string]*elbv2api.IngressClassParams)
	for _, ic := range classes {
		if ic.Spec.Parameters == nil {
			continue
		}
		if ic.Spec.Parameters.Kind == sharedconstants.IngressClassParamsKind {
			if icp, ok := paramsByName[ic.Spec.Parameters.Name]; ok {
				result[ic.Name] = icp
			}
		}
	}
	return result
}
