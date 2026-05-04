package console

import (
	"context"
	"fmt"

	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	ingressDryRunPlanAnnotation = annotations.AnnotationPrefixIngress + "/dry-run-plan"

	// typeLoadBalancer is the resource type key for LoadBalancer in the stack JSON.
	typeLoadBalancer = "AWS::ElasticLoadBalancingV2::LoadBalancer"
)

// GatewayInfo holds metadata about a discovered Gateway with a dry-run plan.
type GatewayInfo struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	IngressPlanHolder string `json:"ingressPlanHolder"`
	Error             string `json:"error,omitempty"` // non-empty if the ingress plan could not be resolved
	GatewayPlan       string `json:"-"`               // raw JSON, not sent in list response
	IngressPlan       string `json:"-"`               // raw JSON, not sent in list response
}

// DiscoverGateways finds all Gateways in the given namespace that have the
// dry-run-plan annotation, and resolves their corresponding ingress sources.
func DiscoverGateways(ctx context.Context, k8sClient client.Client, namespace string) ([]GatewayInfo, error) {
	gwList := &gwv1.GatewayList{}
	if err := k8sClient.List(ctx, gwList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list Gateways in namespace %s: %w", namespace, err)
	}

	var results []GatewayInfo
	for i := range gwList.Items {
		gw := &gwList.Items[i]
		gwPlan, hasPlan := gw.Annotations[gateway_constants.AnnotationDryRunPlan]
		if !hasPlan || gwPlan == "" {
			continue
		}

		info := GatewayInfo{
			Name:        gw.Name,
			Namespace:   gw.Namespace,
			GatewayPlan: gwPlan,
		}

		// Resolve ingress source for group ingress case.
		ingressRef, hasHolder := gw.Annotations[gateway_constants.AnnotationIngressPlanHolder]
		if hasHolder && ingressRef != "" {
			info.IngressPlanHolder = ingressRef
		}

		// Fallback for standalone ingresses: derive ingress source from the
		// gateway model's migrated-from tag (format: "ingress/namespace/name").
		if info.IngressPlanHolder == "" {
			if ref := inferIngressSourceFromPlan(gwPlan); ref != "" {
				info.IngressPlanHolder = ref
			}
		}

		// Try to read the ingress plan.
		if info.IngressPlanHolder == "" {
			info.Error = "could not determine ingress plan holder: no ingress-plan-holder annotation and no migrated-from tag found in gateway model"
		} else {
			ingressPlan, err := readIngressPlan(ctx, k8sClient, info.IngressPlanHolder)
			if err != nil {
				info.Error = fmt.Sprintf("failed to read ingress plan from %s: %v", info.IngressPlanHolder, err)
			} else {
				info.IngressPlan = ingressPlan
			}
		}

		results = append(results, info)
	}

	return results, nil
}

// LoadGatewayInfo loads a single Gateway's dry-run data.
func LoadGatewayInfo(ctx context.Context, k8sClient client.Client, namespace, gatewayName string) (*GatewayInfo, error) {
	gw := &gwv1.Gateway{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: gatewayName}, gw); err != nil {
		return nil, fmt.Errorf("failed to get Gateway %s/%s: %w", namespace, gatewayName, err)
	}

	gwPlan, hasPlan := gw.Annotations[gateway_constants.AnnotationDryRunPlan]
	if !hasPlan || gwPlan == "" {
		return nil, fmt.Errorf("Gateway %s/%s does not have a dry-run-plan annotation", namespace, gatewayName)
	}

	info := &GatewayInfo{
		Name:        gw.Name,
		Namespace:   gw.Namespace,
		GatewayPlan: gwPlan,
	}

	ingressRef, hasHolder := gw.Annotations[gateway_constants.AnnotationIngressPlanHolder]
	if hasHolder && ingressRef != "" {
		info.IngressPlanHolder = ingressRef
	}

	// Fallback for standalone ingresses.
	if info.IngressPlanHolder == "" {
		if ref := inferIngressSourceFromPlan(gwPlan); ref != "" {
			info.IngressPlanHolder = ref
		}
	}

	if info.IngressPlanHolder == "" {
		info.Error = "could not determine ingress plan holder: no ingress-plan-holder annotation and no migrated-from tag found in gateway model"
	} else {
		ingressPlan, err := readIngressPlan(ctx, k8sClient, info.IngressPlanHolder)
		if err != nil {
			info.Error = fmt.Sprintf("failed to read ingress plan from %s: %v", info.IngressPlanHolder, err)
		} else {
			info.IngressPlan = ingressPlan
		}
	}

	return info, nil
}

// readIngressPlan reads the dry-run-plan annotation from an Ingress.
// ingressRef is in "namespace/name" format.
func readIngressPlan(ctx context.Context, k8sClient client.Client, ingressRef string) (string, error) {
	ns, name, err := parseNamespacedName(ingressRef)
	if err != nil {
		return "", err
	}

	ing := &networking.Ingress{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, ing); err != nil {
		return "", fmt.Errorf("failed to get Ingress %s: %w", ingressRef, err)
	}

	plan, ok := ing.Annotations[ingressDryRunPlanAnnotation]
	if !ok || plan == "" {
		return "", fmt.Errorf("Ingress %s does not have a dry-run-plan annotation", ingressRef)
	}
	return plan, nil
}

// parseNamespacedName splits "namespace/name" into its parts.
func parseNamespacedName(ref string) (string, string, error) {
	for i, c := range ref {
		if c == '/' {
			ns := ref[:i]
			name := ref[i+1:]
			if ns == "" || name == "" {
				return "", "", fmt.Errorf("invalid namespaced name: %q", ref)
			}
			return ns, name, nil
		}
	}
	return "", "", fmt.Errorf("invalid namespaced name (missing /): %q", ref)
}

// inferIngressSourceFromPlan parses the gateway model JSON to find the
// migrated-from tag on the LoadBalancer resource. The tag format is
// "ingress/namespace/name" for standalone ingresses.
func inferIngressSourceFromPlan(planJSON string) string {
	tree, err := ParseStack(planJSON)
	if err != nil {
		return ""
	}
	lbResources, ok := tree[typeLoadBalancer]
	if !ok {
		return ""
	}
	for _, fields := range lbResources {
		tag, ok := fields["spec.tags.gateway.k8s.aws/migrated-from"].(string)
		if !ok {
			continue
		}
		// Format: "ingress/namespace/name" → return "namespace/name"
		const prefix = "ingress/"
		if len(tag) > len(prefix) && tag[:len(prefix)] == prefix {
			return tag[len(prefix):]
		}
	}
	return ""
}
