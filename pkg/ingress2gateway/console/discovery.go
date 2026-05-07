package console

import (
	"context"
	"fmt"
	"sort"
	"strings"

	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	ingressDryRunPlanAnnotation = annotations.AnnotationPrefixIngress + "/" + annotations.IngressSuffixDryRunPlan
	ingressGroupNameAnnotation  = annotations.AnnotationPrefixIngress + "/" + annotations.IngressSuffixGroupName

	migratedFromIngressPrefix      = "ingress/"
	migratedFromIngressGroupPrefix = "ingress-group/"
)

// GatewayInfo holds metadata about a discovered Gateway with a dry-run plan.
type GatewayInfo struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Error       string `json:"error,omitempty"` // non-empty if the ingress plan could not be resolved
	GatewayPlan string `json:"-"`               // raw JSON, not sent in list response
	IngressPlan string `json:"-"`               // raw JSON, not sent in list response
}

// NamespaceInfo describes a namespace that has at least one Gateway with a dry-run-plan annotation.
type NamespaceInfo struct {
	Namespace    string `json:"namespace"`
	GatewayCount int    `json:"gatewayCount"`
}

// DiscoverNamespaces lists all Gateways cluster-wide, finds the ones with the
// dry-run-plan annotation, and groups them by namespace. Returns a sorted list
// of namespaces (alphabetical) with gateway counts.
func DiscoverNamespaces(ctx context.Context, k8sClient client.Client) ([]NamespaceInfo, error) {
	gwList := &gwv1.GatewayList{}
	if err := k8sClient.List(ctx, gwList); err != nil {
		return nil, fmt.Errorf("failed to list Gateways cluster-wide: %w", err)
	}

	countsByNS := map[string]int{}
	for i := range gwList.Items {
		gw := &gwList.Items[i]
		plan, hasPlan := gw.Annotations[gateway_constants.AnnotationDryRunPlan]
		if !hasPlan || plan == "" {
			continue
		}
		countsByNS[gw.Namespace]++
	}

	results := make([]NamespaceInfo, 0, len(countsByNS))
	for ns, count := range countsByNS {
		results = append(results, NamespaceInfo{Namespace: ns, GatewayCount: count})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Namespace < results[j].Namespace })
	return results, nil
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
		results = append(results, resolveGatewayInfo(ctx, k8sClient, gw, gwPlan))
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

	info := resolveGatewayInfo(ctx, k8sClient, gw, gwPlan)
	return &info, nil
}

// resolveGatewayInfo derives the ingress source for a single Gateway and fetches
// its plan annotation. We resolve the source purely from the migrated-from tag on
// the LoadBalancer resource in the gateway plan:
//   - "ingress/ns/name"       → direct pointer to that ingress
//   - "ingress-group/<name>"  → list ingresses filtered by group.name annotation
//     and return the one whose dry-run-plan annotation is non-empty
func resolveGatewayInfo(ctx context.Context, k8sClient client.Client, gw *gwv1.Gateway, gwPlan string) GatewayInfo {
	info := GatewayInfo{
		Name:        gw.Name,
		Namespace:   gw.Namespace,
		GatewayPlan: gwPlan,
	}

	tag := readMigratedFromTag(gwPlan)
	if tag == "" {
		info.Error = "could not determine ingress plan holder: no migrated-from tag found on LoadBalancer in gateway model"
		return info
	}

	holderRef, err := resolvePlanHolder(ctx, k8sClient, gw.Namespace, tag)
	if err != nil {
		info.Error = err.Error()
		return info
	}

	plan, err := readIngressPlan(ctx, k8sClient, holderRef)
	if err != nil {
		info.Error = fmt.Sprintf("failed to read ingress plan from %s: %v", holderRef, err)
		return info
	}
	info.IngressPlan = plan
	return info
}

// resolvePlanHolder maps a migrated-from tag to the "namespace/name" of the ingress
// that holds the dry-run plan. For standalone ingresses the tag already contains
// the namespaced name; for groups we discover the holder by scanning members for
// the plan annotation.
//
// Returns an error if no holder can be found, or if the group has multiple
// ingresses with a plan annotation (an indicator of a stale-annotation leak
// across reconciles; the controller now cleans these up but older clusters
// migrated before the cleanup was added may still trip this path).
func resolvePlanHolder(ctx context.Context, k8sClient client.Client, gwNamespace, tag string) (string, error) {
	if strings.HasPrefix(tag, migratedFromIngressPrefix) {
		return strings.TrimPrefix(tag, migratedFromIngressPrefix), nil
	}

	if !strings.HasPrefix(tag, migratedFromIngressGroupPrefix) {
		return "", fmt.Errorf("unrecognized migrated-from tag %q: expected prefix %q or %q",
			tag, migratedFromIngressPrefix, migratedFromIngressGroupPrefix)
	}
	groupName := strings.TrimPrefix(tag, migratedFromIngressGroupPrefix)
	if groupName == "" {
		return "", fmt.Errorf("migrated-from tag carries empty ingress-group name")
	}

	// Explicit groups can span namespaces, so we list cluster-wide. This is
	// only called from the console on user action, so the extra list traffic
	// is acceptable — it's not on the reconcile hot path.
	ingList := &networking.IngressList{}
	if err := k8sClient.List(ctx, ingList); err != nil {
		return "", fmt.Errorf("failed to list ingresses for group %q: %w", groupName, err)
	}

	var holders []string
	for i := range ingList.Items {
		ing := &ingList.Items[i]
		if ing.Annotations[ingressGroupNameAnnotation] != groupName {
			continue
		}
		if plan := ing.Annotations[ingressDryRunPlanAnnotation]; plan != "" {
			holders = append(holders, fmt.Sprintf("%s/%s", ing.Namespace, ing.Name))
		}
	}

	// Deterministic ordering makes the multi-holder error message readable
	// and keeps unit tests stable even when the fake client returns items
	// in a different order.
	sort.Strings(holders)

	switch len(holders) {
	case 0:
		return "", fmt.Errorf("no ingress in group %q carries a dry-run-plan annotation; the ingress controller has not yet reconciled the group, or the IngressPlanAnnotation feature gate is disabled", groupName)
	case 1:
		return holders[0], nil
	default:
		return "", fmt.Errorf("multiple ingresses in group %q carry a dry-run-plan annotation: %s; this usually means a stale annotation was left behind after group membership changed — manually clear the annotation from all but one member", groupName, strings.Join(holders, ", "))
	}
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
	idx := strings.IndexByte(ref, '/')
	if idx <= 0 || idx == len(ref)-1 {
		return "", "", fmt.Errorf("invalid namespaced name: %q", ref)
	}
	return ref[:idx], ref[idx+1:], nil
}

// readMigratedFromTag pulls the migrated-from tag from the LoadBalancer resource
// in the gateway plan JSON. Returns "" if parsing fails or the tag is absent.
func readMigratedFromTag(planJSON string) string {
	tree, err := ParseStack(planJSON)
	if err != nil {
		return ""
	}
	lbResources, ok := tree[utils.StackResTypeLoadBalancer]
	if !ok {
		return ""
	}
	for _, fields := range lbResources {
		if tag, ok := fields[migratedFromTagField].(string); ok && tag != "" {
			return tag
		}
	}
	return ""
}
