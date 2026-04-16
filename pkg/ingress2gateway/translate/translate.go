package translate

import (
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	k8s "sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
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

	// Partition Ingresses into groups (ungrouped become single-member groups)
	groups := partitionByGroup(in.Ingresses)

	for _, group := range groups {
		var gatewayName, lbConfigName, lbMigrationTag string
		if group.isExplicit {
			gatewayName = utils.GetGroupGatewayName(group.name)
			lbConfigName = utils.GetGroupLBConfigName(group.name)
			lbMigrationTag = fmt.Sprintf("ingress-group/%s", group.name)
		} else {
			gatewayName = utils.GetGatewayName(group.namespace, group.name)
			lbConfigName = utils.GetLBConfigName(group.namespace, group.name)
			lbMigrationTag = fmt.Sprintf("ingress/%s/%s", group.namespace, group.name)
		}

		// Warn about cross-namespace groups
		if group.crossNamespace {
			fmt.Fprintf(os.Stderr, utils.WarnCrossNamespaceGroupFormat, group.name)
		}

		// --- Group-level: merge annotations, resolve ports, ICP, ssl-redirect ---
		mergedAnnotations, err := mergeGroupLBAnnotations(group.members)
		if err != nil {
			return nil, fmt.Errorf("error in mergeGroupLBAnnotations for group %q: %w", group.name, err)
		}

		allPorts, perMemberPorts, err := mergeGroupListenPorts(group.members, mergedAnnotations)
		if err != nil {
			return nil, fmt.Errorf("error in mergeGroupListenPorts for group %q: %w", group.name, err)
		}

		icp, err := resolveGroupICP(group.members, ingressClassParamsByClass)
		if err != nil {
			return nil, fmt.Errorf("error in resolveGroupICP for group %q: %w", group.name, err)
		}

		sslRedirectPort, err := resolveGroupSSLRedirect(group.members, icp)
		if err != nil {
			return nil, fmt.Errorf("error in resolveGroupSSLRedirect for group %q: %w", group.name, err)
		}

		// --- Build LoadBalancerConfiguration ---
		lbConfig := buildLoadBalancerConfigResource(lbConfigName, group.namespace, mergedAnnotations, allPorts, lbMigrationTag)
		if icp != nil {
			if lbConfig == nil {
				lbConfig = &gatewayv1beta1.LoadBalancerConfiguration{
					TypeMeta: metav1.TypeMeta{
						APIVersion: utils.LBConfigAPIVersion,
						Kind:       gwconstants.LoadBalancerConfiguration,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      lbConfigName,
						Namespace: group.namespace,
					},
				}
			}
			applyIngressClassParamsToLBConfig(&lbConfig.Spec, icp)
		}
		if lbConfig != nil {
			out.LoadBalancerConfigurations = append(out.LoadBalancerConfigurations, *lbConfig)
		}

		// --- Build Gateway ---
		crossNSGroupName := ""
		if group.crossNamespace {
			crossNSGroupName = group.name
		}
		gw := buildGateway(gatewayName, group.namespace, lbConfig, allPorts, crossNSGroupName)
		out.Gateways = append(out.Gateways, gw)

		// --- SSL redirect route ---
		if sslRedirectPort != nil {
			for _, lp := range allPorts {
				if strings.EqualFold(lp.Protocol, utils.ProtocolHTTP) {
					redirectRoute := buildSSLRedirectRoute(
						group.namespace, group.name, gatewayName,
						utils.GenerateSectionName(lp.Protocol, lp.Port),
						*sslRedirectPort,
					)
					out.HTTPRoutes = append(out.HTTPRoutes, redirectRoute)
				}
			}
		}

		// --- Per-member: HTTPRoutes, TGConfigs, LRConfigs ---
		for _, ing := range group.members {
			memberPorts := perMemberPorts[k8s.NamespacedName(&ing).String()]
			parentRefs := buildMemberParentRefs(gatewayName, group.namespace, ing.Namespace, memberPorts, allPorts, sslRedirectPort)

			routes, svcRefs, lrcs, err := buildHTTPRoutes(ing, ing.Namespace, parentRefs, servicesByKey)
			if err != nil {
				return nil, err
			}
			out.HTTPRoutes = append(out.HTTPRoutes, routes...)

			// Add migration tags to ListenerRuleConfigurations
			memberMigrationTag := fmt.Sprintf("ingress/%s/%s", ing.Namespace, ing.Name)
			for i := range lrcs {
				if lrcs[i].Spec.Tags == nil {
					tags := make(map[string]string)
					lrcs[i].Spec.Tags = &tags
				}
				(*lrcs[i].Spec.Tags)[utils.MigrationTagKey] = memberMigrationTag
			}
			out.ListenerRuleConfigurations = append(out.ListenerRuleConfigurations, lrcs...)

			// Build TargetGroupConfigurations for each unique service
			effectiveAnnotations := resolveAnnotations(ing)
			for _, svcRef := range svcRefs {
				if tgcCreated.Has(svcRef.getServiceRefKey()) {
					continue
				}
				tgcCreated.Insert(svcRef.getServiceRefKey())

				tgAnnotations := mergeTGAnnotations(effectiveAnnotations, servicesByKey, svcRef.namespace, svcRef.name)
				tgc := buildTargetGroupConfig(svcRef, tgAnnotations, memberMigrationTag)
				if tgc != nil {
					if icp != nil {
						applyIngressClassParamsToTGProps(&tgc.Spec.DefaultConfiguration, icp)
					}
					out.TargetGroupConfigurations = append(out.TargetGroupConfigurations, *tgc)
				}
			}
		}
	}

	return out, nil
}

// resolveAnnotations returns the Ingress annotations map.
func resolveAnnotations(ing networking.Ingress) map[string]string {
	output := make(map[string]string)
	for k, v := range ing.Annotations {
		output[k] = v
	}
	return output
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
		serviceMap[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = svc
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
