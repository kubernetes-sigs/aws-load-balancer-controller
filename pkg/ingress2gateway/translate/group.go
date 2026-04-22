package translate

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	networking "k8s.io/api/networking/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	k8s "sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ingressAnnotationKeyPrefix is the full annotation key prefix including the trailing slash.
const ingressAnnotationKeyPrefix = annotations.AnnotationPrefixIngress + "/"

// ingressGroup represents a set of Ingresses that share one ALB.
// For explicit groups (group.name annotation), multiple Ingresses share one Gateway.
// For implicit groups (no group.name), each Ingress is a group of 1, which means it has 1 member
type ingressGroup struct {
	name           string // group.name for explicit, Ingress name for implicit
	namespace      string // first member's namespace (Gateway placement)
	isExplicit     bool
	crossNamespace bool
	members        []networking.Ingress
}

// partitionByGroup splits Ingresses into groups. Ingresses with the same
// group.name annotation form an explicit group. Ingresses without group.name
// become implicit single-member groups.
func partitionByGroup(ingresses []networking.Ingress) []ingressGroup {
	explicit := make(map[string][]networking.Ingress)
	var ungrouped []networking.Ingress

	for _, ing := range ingresses {
		groupName := getString(ing.Annotations, annotations.IngressSuffixGroupName)
		if groupName != "" {
			explicit[groupName] = append(explicit[groupName], ing)
		} else {
			ungrouped = append(ungrouped, ing)
		}
	}

	var groups []ingressGroup

	for _, name := range slices.Sorted(maps.Keys(explicit)) {
		members := explicit[name]
		crossNS := isCrossNamespace(members)
		groups = append(groups, ingressGroup{
			name:           name,
			namespace:      members[0].Namespace,
			isExplicit:     true,
			crossNamespace: crossNS,
			members:        members,
		})
	}

	for _, ing := range ungrouped {
		groups = append(groups, ingressGroup{
			name:      ing.Name,
			namespace: ing.Namespace,
			members:   []networking.Ingress{ing},
		})
	}

	return groups
}

// isCrossNamespace returns true if ingresses span multiple namespaces.
func isCrossNamespace(ingresses []networking.Ingress) bool {
	if len(ingresses) <= 1 {
		return false
	}
	firstNS := ingresses[0].Namespace
	for _, m := range ingresses[1:] {
		if m.Namespace != firstNS {
			return true
		}
	}
	return false
}

// lbLevelAnnotationSuffixes are the annotation suffixes that must be consistent
// across all Ingresses in a group. LBC errors on conflict at runtime for these.
var lbLevelAnnotationSuffixes = []string{
	annotations.IngressSuffixScheme,
	annotations.IngressSuffixLoadBalancerName,
	annotations.IngressSuffixIPAddressType,
	annotations.IngressSuffixSubnets,
	annotations.IngressSuffixCustomerOwnedIPv4Pool,
	annotations.IngressSuffixIPAMIPv4PoolId,
	annotations.IngressSuffixSecurityGroups,
	annotations.IngressSuffixManageSecurityGroupRules,
	annotations.IngressSuffixInboundCIDRs,
	annotations.IngressSuffixSecurityGroupPrefixLists,
	annotations.IngressSuffixSSLPolicy,
	annotations.IngressSuffixWAFv2ACLARN,
	annotations.IngressSuffixWAFv2ACLName,
	annotations.IngressSuffixShieldAdvancedProtection,
	annotations.IngressSuffixLoadBalancerCapacityReservation,
	annotations.IngressSuffixMutualAuthentication,
}

// mergeGroupLBAnnotations merges LB-level annotations across group members.
// Scalar annotations: error if >1 distinct value.
// certificate-arn: union. tags / load-balancer-attributes: union keys, error on per-key conflict.
func mergeGroupLBAnnotations(members []networking.Ingress) (map[string]string, error) {
	if len(members) == 1 {
		return resolveAnnotations(members[0]), nil
	}

	merged := make(map[string]string)

	// Scalar LB-level annotations: error on conflict
	for _, suffix := range lbLevelAnnotationSuffixes {
		if err := mergeExactMatchAnnotation(members, merged, suffix); err != nil {
			return nil, err
		}
	}

	if err := mergeCertificateARNs(members, merged); err != nil {
		return nil, err
	}
	if err := mergeStringMapAnnotation(members, merged, annotations.IngressSuffixTags); err != nil {
		return nil, err
	}
	if err := mergeStringMapAnnotation(members, merged, annotations.IngressSuffixLoadBalancerAttributes); err != nil {
		return nil, err
	}

	return merged, nil
}

// mergeExactMatchAnnotation merges an annotation that must have the same value
// across all members. Errors if two members set different non-empty values.
func mergeExactMatchAnnotation(members []networking.Ingress, merged map[string]string, suffix string) error {
	key := ingressAnnotationKeyPrefix + suffix
	var annotationVal string
	set := false
	for _, ing := range members {
		value, exists := ing.Annotations[key]
		if !exists || value == "" {
			continue
		}
		if !set {
			annotationVal = value
			set = true
		} else if value != annotationVal {
			return fmt.Errorf("conflicting annotation %s in group: %q vs %q",
				key, annotationVal, value)
		}
	}
	if set {
		merged[key] = annotationVal
	}
	return nil
}

// mergeCertificateARNs unions certificate-arn values across members.
func mergeCertificateARNs(members []networking.Ingress, merged map[string]string) error {
	key := ingressAnnotationKeyPrefix + annotations.IngressSuffixCertificateARN
	seen := make(map[string]struct{}) // for dedupe
	var ordered []string              // for sorted output
	for _, ing := range members {
		value, exists := ing.Annotations[key]
		if !exists || value == "" {
			continue
		}
		for _, cert := range strings.Split(value, ",") {
			cert = strings.TrimSpace(cert)
			if cert == "" {
				continue
			}
			if _, exists := seen[cert]; !exists {
				seen[cert] = struct{}{}
				ordered = append(ordered, cert)
			}
		}
	}
	if len(ordered) > 0 {
		merged[key] = strings.Join(ordered, ",")
	}
	return nil
}

// mergeStringMapAnnotation merges a stringMap annotation (k1=v1,k2=v2) across members.
// Union keys; error if same key has different values.
func mergeStringMapAnnotation(members []networking.Ingress, merged map[string]string, suffix string) error {
	key := ingressAnnotationKeyPrefix + suffix
	unionMap := make(map[string]string)
	for _, ing := range members {
		var parsed map[string]string
		if _, err := ingressAnnotationParser.ParseStringMapAnnotation(suffix, &parsed, ing.Annotations); err != nil {
			return fmt.Errorf("failed to parse %s on %s: %w", key, k8s.NamespacedName(&ing).String(), err)
		}
		for mk, mv := range parsed {
			if existing, exists := unionMap[mk]; exists && existing != mv {
				return fmt.Errorf("conflicting %s key %q in group: %q vs %q", suffix, mk, existing, mv)
			}
			unionMap[mk] = mv
		}
	}
	if len(unionMap) > 0 {
		var parts []string
		for k, v := range unionMap {
			parts = append(parts, k+"="+v)
		}
		sort.Strings(parts)
		merged[key] = strings.Join(parts, ",")
	}
	return nil
}

// mergeGroupListenPorts unions listen-port entries across group members.
// Returns allPorts (the union for the shared Gateway) and perIngressPorts
func mergeGroupListenPorts(members []networking.Ingress) ([]listenPortEntry, map[string][]listenPortEntry, error) {
	seen := make(map[listenPortEntry]struct{})
	var allPorts []listenPortEntry
	perIngressPorts := make(map[string][]listenPortEntry)

	for _, ing := range members {
		ports, err := parseListenPorts(ing.Annotations)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse listen-ports for %s: %w", k8s.NamespacedName(&ing).String(), err)
		}
		if len(ports) == 0 {
			ports = defaultListenPorts(ing.Annotations)
		}
		perIngressPorts[k8s.NamespacedName(&ing).String()] = ports
		for _, p := range ports {
			pk := listenPortEntry{Protocol: p.Protocol, Port: p.Port}
			if _, exists := seen[pk]; !exists {
				seen[pk] = struct{}{}
				allPorts = append(allPorts, p)
			}
		}
	}

	sort.Slice(allPorts, func(i, j int) bool {
		if allPorts[i].Port != allPorts[j].Port {
			return allPorts[i].Port < allPorts[j].Port
		}
		return allPorts[i].Protocol < allPorts[j].Protocol
	})

	return allPorts, perIngressPorts, nil
}

// defaultListenPorts returns the default listen-port when no listen-ports annotation is set.
// HTTPS:443 if certificate-arn is present on the member itself, otherwise HTTP:80.
func defaultListenPorts(ingressAnnotation map[string]string) []listenPortEntry {
	if getString(ingressAnnotation, annotations.IngressSuffixCertificateARN) != "" {
		return []listenPortEntry{{Protocol: utils.ProtocolHTTPS, Port: 443}}
	}
	return []listenPortEntry{{Protocol: utils.ProtocolHTTP, Port: 80}}
}

// resolveGroupICPs collects all unique IngressClassParams referenced by group members.
// In LBC, each member's ICP values are collected into per-field sets and conflict-checked
// at the value level. We return all unique ICPs so the caller can apply them all and
// let the per-field conflict detection in applyIngressClassParamsToLBConfig catch issues.
func resolveGroupICPs(members []networking.Ingress, icpByClass map[string]*elbv2api.IngressClassParams) []*elbv2api.IngressClassParams {
	seen := make(map[string]struct{})
	var icps []*elbv2api.IngressClassParams
	for _, ing := range members {
		if ing.Spec.IngressClassName == nil {
			continue
		}
		icp, exist := icpByClass[*ing.Spec.IngressClassName]
		if !exist {
			continue
		}
		if _, exists := seen[icp.Name]; !exists {
			seen[icp.Name] = struct{}{}
			icps = append(icps, icp)
		}
	}
	return icps
}

// resolveGroupSSLRedirect collects ssl-redirect across all members and ICPs.
// Returns nil if not set. Errors if >1 distinct value.
func resolveGroupSSLRedirect(members []networking.Ingress, icpByClass map[string]*elbv2api.IngressClassParams) (*int32, error) {
	// Per-member resolution: ICP wins over annotation for each member.
	// Then group-wide conflict detection across all resolved values.
	// This matches LBC's buildSSLRedirectConfig behavior.
	var result *int32
	for _, ing := range members {
		var p *int32

		// Check if this member's ICP sets ssl-redirect
		if ing.Spec.IngressClassName != nil {
			if icp, exist := icpByClass[*ing.Spec.IngressClassName]; exist && icp.Spec.SSLRedirectPort != "" {
				port, err := strconv.ParseInt(icp.Spec.SSLRedirectPort, 10, 32)
				if err != nil {
					return nil, fmt.Errorf("invalid ICP ssl-redirect port %q: %w", icp.Spec.SSLRedirectPort, err)
				}
				v := int32(port)
				p = &v
			}
		}

		// If ICP didn't set it for this member, check annotation
		if p == nil {
			p = getInt32(ing.Annotations, annotations.IngressSuffixSSLRedirect)
		}

		if p == nil {
			continue
		}
		if result == nil {
			result = p
		} else if *p != *result {
			return nil, fmt.Errorf("conflicting ssl-redirect ports in group: %d vs %d", *result, *p)
		}
	}
	return result, nil
}

// listenPortsEqual returns true if two port slices contain the same entries (order-independent).
func listenPortsEqual(a, b []listenPortEntry) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[listenPortEntry]struct{}, len(a))
	for _, p := range a {
		set[listenPortEntry{Protocol: p.Protocol, Port: p.Port}] = struct{}{}
	}
	for _, p := range b {
		if _, ok := set[listenPortEntry{Protocol: p.Protocol, Port: p.Port}]; !ok {
			return false
		}
	}
	return true
}

// buildMemberParentRefs builds parentRefs for a member's HTTPRoutes.
// - 1. sslRedirectPort set: scope to HTTPS listener only.
// - 2. memberPorts == allPorts: no sectionName (attach to all listeners).
// - 3. Otherwise: one parentRef per memberPort with sectionName.
func buildMemberParentRefs(gatewayName, gatewayNamespace, memberNamespace string, memberPorts, allPorts []listenPortEntry, sslRedirectPort *int32) []gwv1.ParentReference {
	if sslRedirectPort != nil {
		sn := utils.GenerateSectionName(utils.ProtocolHTTPS, *sslRedirectPort)
		return []gwv1.ParentReference{buildParentRef(gatewayName, gatewayNamespace, memberNamespace, &sn)}
	}
	if listenPortsEqual(memberPorts, allPorts) {
		return []gwv1.ParentReference{buildParentRef(gatewayName, gatewayNamespace, memberNamespace, nil)}
	}
	refs := make([]gwv1.ParentReference, 0, len(memberPorts))
	for _, listenerPortEntry := range memberPorts {
		sn := utils.GenerateSectionName(listenerPortEntry.Protocol, listenerPortEntry.Port)
		refs = append(refs, buildParentRef(gatewayName, gatewayNamespace, memberNamespace, &sn))
	}
	return refs
}

// buildParentRef creates a single ParentReference, adding namespace if cross-namespace.
func buildParentRef(gatewayName, gatewayNamespace, memberNamespace string, sectionName *string) gwv1.ParentReference {
	ref := gwv1.ParentReference{
		Name: gwv1.ObjectName(gatewayName),
	}
	if sectionName != nil {
		sn := gwv1.SectionName(*sectionName)
		ref.SectionName = &sn
	}
	if gatewayNamespace != memberNamespace {
		ns := gwv1.Namespace(gatewayNamespace)
		ref.Namespace = &ns
	}
	return ref
}
