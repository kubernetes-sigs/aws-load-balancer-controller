package translate

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
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
	tgcCreated := sets.New[string]()

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
		if icp != nil {
			if lbConfig == nil {
				lbConfig = &gatewayv1beta1.LoadBalancerConfiguration{
					TypeMeta: metav1.TypeMeta{
						APIVersion: utils.LBConfigAPIVersion,
						Kind:       gwconstants.LoadBalancerConfiguration,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      lbConfigName,
						Namespace: namespace,
					},
				}
			}
			applyIngressClassParamsToLBConfig(&lbConfig.Spec, icp)
		}

		if lbConfig != nil {
			out.LoadBalancerConfigurations = append(out.LoadBalancerConfigurations, *lbConfig)
		}

		// Build Gateway
		gw := buildGateway(gatewayName, namespace, lbConfig, listenPorts)
		out.Gateways = append(out.Gateways, gw)

		// Build HTTPRoute(s), collect unique service refs, and collect ListenerRuleConfigurations
		sslRedirectPort := resolveSSLRedirectPort(ing.Annotations, icp)
		routes, svcRefs, lrcs, err := buildHTTPRoutes(ing, namespace, gatewayName, listenPorts, servicesByKey, sslRedirectPort)
		if err != nil {
			return nil, err
		}
		out.HTTPRoutes = append(out.HTTPRoutes, routes...)
		// Add migration tags to listenerRuleConfig
		for i := range lrcs {
			if lrcs[i].Spec.Tags == nil {
				tags := make(map[string]string)
				lrcs[i].Spec.Tags = &tags
			}
			(*lrcs[i].Spec.Tags)[utils.MigrationTagKey] = migrationTag
		}
		out.ListenerRuleConfigurations = append(out.ListenerRuleConfigurations, lrcs...)

		// Build TargetGroupConfigurations for each unique service
		for _, svcRef := range svcRefs {
			if tgcCreated.Has(svcRef.getServiceRefKey()) {
				continue
			}
			tgcCreated.Insert(svcRef.getServiceRefKey())

			tgAnnotations := mergeTGAnnotations(effectiveAnnotations, servicesByKey, svcRef.namespace, svcRef.name)
			tgc := buildTargetGroupConfig(svcRef, tgAnnotations, migrationTag)
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

// mergeTGAnnotations merges service-level annotations over ingress annotations.
// Service annotations take priority over Ingress annotations for per-backend fields.
// The merged result is only consumed by buildTargetGroupConfig (and future per-backend
// builders like auth/LRC), so any extra Service annotations are harmlessly ignored.
func mergeTGAnnotations(ingressAnnotations map[string]string, servicesByKey map[string]corev1.Service, namespace, svcName string) map[string]string {
	merged := make(map[string]string)
	for k, v := range ingressAnnotations {
		merged[k] = v
	}

	svcKey := fmt.Sprintf("%s/%s", namespace, svcName)
	if svc, ok := servicesByKey[svcKey]; ok {
		for k, v := range svc.Annotations {
			merged[k] = v
		}
	}

	return merged
}

// serviceRef holds a service reference extracted from an Ingress backend.
type serviceRef struct {
	namespace string
	name      string
	port      int32
}

// getServiceRefKey returns a unique string getServiceRefKey for deduplication.
func (s serviceRef) getServiceRefKey() string {
	return fmt.Sprintf("%s/%s:%d", s.namespace, s.name, s.port)
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
