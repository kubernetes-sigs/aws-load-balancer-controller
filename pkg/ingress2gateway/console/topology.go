package console

import (
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

// TopologyNode represents a single resource in the resource map.
type TopologyNode struct {
	ID           string `json:"id"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	Label        string `json:"label"`
	Status       string `json:"status"`
}

// TopologyEdge represents a directional relationship between two resources.
type TopologyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// TopologyResult holds the full resource graph for a gateway's stack.
type TopologyResult struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// BuildTopology constructs a resource graph from the gateway ResourceTree and
// annotates each node with its aggregate diff status from the DiffResult.
func BuildTopology(gatewayTree ResourceTree, diff DiffResult) TopologyResult {
	var nodes []TopologyNode
	var edges []TopologyEdge

	nodeStatuses := aggregateNodeStatuses(diff)

	// Collect all resource nodes.
	for resType, resources := range gatewayTree {
		for rawID := range resources {
			nodeID := resType + "|" + rawID
			label := shortLabel(resType, rawID, gatewayTree)
			status := nodeStatuses[nodeID]
			if status == "" {
				status = "same"
			}
			nodes = append(nodes, TopologyNode{
				ID:           nodeID,
				ResourceType: resType,
				ResourceID:   rawID,
				Label:        label,
				Status:       status,
			})
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	// Build edges from resource references.
	edges = append(edges, buildListenerToLBEdges(gatewayTree)...)
	edges = append(edges, buildRuleToListenerEdges(gatewayTree)...)
	edges = append(edges, buildRuleToTargetGroupEdges(gatewayTree)...)
	edges = append(edges, buildTGBToTargetGroupEdges(gatewayTree)...)
	edges = append(edges, buildLBToSecurityGroupEdges(gatewayTree)...)

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	return TopologyResult{Nodes: nodes, Edges: edges}
}

// aggregateNodeStatuses determines the overall status for each resource node
// based on its field-level diff entries. If any field is changed/added/removed,
// the node takes the most severe status.
func aggregateNodeStatuses(diff DiffResult) map[string]string {
	statuses := make(map[string]string)
	for _, e := range diff.Entries {
		nodeID := e.ResourceType + "|" + e.CorrelationID
		if e.GatewayResourceID != "" {
			nodeID = e.ResourceType + "|" + e.GatewayResourceID
		}
		current := statuses[nodeID]
		statuses[nodeID] = mergeStatus(current, string(e.Status))
	}
	return statuses
}

var statusPriority = map[string]int{"same": 0, "added": 1, "removed": 2, "changed": 3}

// mergeStatus returns the more severe of two statuses.
func mergeStatus(a, b string) string {
	if statusPriority[b] > statusPriority[a] {
		return b
	}
	return a
}

// shortLabel derives a concise display label for a node.
func shortLabel(resType, rawID string, tree ResourceTree) string {
	fields := tree[resType][rawID]
	switch resType {
	case utils.StackResTypeLoadBalancer:
		if name, ok := fields["spec.name"].(string); ok {
			return name
		}
	case utils.StackResTypeListener:
		if port, ok := fields["spec.port"]; ok {
			return fmt.Sprintf(":%v", port)
		}
	case utils.StackResTypeListenerRule:
		parts := strings.Split(rawID, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	case utils.StackResTypeTargetGroup:
		if name, ok := fields["spec.name"].(string); ok {
			return name
		}
	case utils.StackResTypeTargetGroupBinding:
		svcName, _ := fields["spec.template.spec.serviceRef.name"].(string)
		svcPort := fields["spec.template.spec.serviceRef.port"]
		if svcName != "" && svcPort != nil {
			return fmt.Sprintf("%s:%v", svcName, svcPort)
		}
	case utils.StackResTypeSecurityGroup:
		if name, ok := fields["spec.groupName"].(string); ok {
			return name
		}
	}
	if len(rawID) > 40 {
		return "…" + rawID[len(rawID)-38:]
	}
	return rawID
}

// buildListenerToLBEdges connects Listeners to the LoadBalancer.
// In a typical stack there is one LB; all Listeners are its children.
func buildListenerToLBEdges(tree ResourceTree) []TopologyEdge {
	lbResources := tree[utils.StackResTypeLoadBalancer]
	if len(lbResources) == 0 {
		return nil
	}
	// Find the LB ID (usually just one).
	var lbID string
	for id := range lbResources {
		lbID = id
		break
	}
	lbNodeID := utils.StackResTypeLoadBalancer + "|" + lbID

	var edges []TopologyEdge
	for listenerID := range tree[utils.StackResTypeListener] {
		edges = append(edges, TopologyEdge{
			From: lbNodeID,
			To:   utils.StackResTypeListener + "|" + listenerID,
		})
	}
	return edges
}

// buildRuleToListenerEdges connects ListenerRules to their parent Listener
// by parsing spec.listenerARN.$ref fields.
func buildRuleToListenerEdges(tree ResourceTree) []TopologyEdge {
	var edges []TopologyEdge
	for ruleID, fields := range tree[utils.StackResTypeListenerRule] {
		ruleNodeID := utils.StackResTypeListenerRule + "|" + ruleID
		for key, val := range fields {
			if key == "spec.listenerARN.$ref" {
				if targetID := parseRef(val, utils.StackResTypeListener); targetID != "" {
					edges = append(edges, TopologyEdge{
						From: utils.StackResTypeListener + "|" + targetID,
						To:   ruleNodeID,
					})
				}
			}
		}
	}
	return edges
}

// buildRuleToTargetGroupEdges connects ListenerRules to TargetGroups.
// The targetGroupARN.$ref can appear either as a flat key (if the model
// structure is map-only) or nested inside array values (spec.actions is
// serialized as []Action). We search both flat keys and array contents.
func buildRuleToTargetGroupEdges(tree ResourceTree) []TopologyEdge {
	seen := make(map[string]bool)
	var edges []TopologyEdge
	for ruleID, fields := range tree[utils.StackResTypeListenerRule] {
		ruleNodeID := utils.StackResTypeListenerRule + "|" + ruleID
		refs := findAllRefs(fields, utils.StackResTypeTargetGroup)
		for _, targetID := range refs {
			edgeKey := ruleNodeID + "->" + targetID
			if !seen[edgeKey] {
				seen[edgeKey] = true
				edges = append(edges, TopologyEdge{
					From: ruleNodeID,
					To:   utils.StackResTypeTargetGroup + "|" + targetID,
				})
			}
		}
	}
	return edges
}

// buildTGBToTargetGroupEdges connects TargetGroupBindings to their TargetGroup.
func buildTGBToTargetGroupEdges(tree ResourceTree) []TopologyEdge {
	var edges []TopologyEdge
	for tgbID, fields := range tree[utils.StackResTypeTargetGroupBinding] {
		refs := findAllRefs(fields, utils.StackResTypeTargetGroup)
		for _, targetID := range refs {
			edges = append(edges, TopologyEdge{
				From: utils.StackResTypeTargetGroup + "|" + targetID,
				To:   utils.StackResTypeTargetGroupBinding + "|" + tgbID,
			})
		}
	}
	return edges
}

// buildLBToSecurityGroupEdges connects the LoadBalancer to its SecurityGroups.
// The LB's spec.securityGroups field is an array of $ref objects.
func buildLBToSecurityGroupEdges(tree ResourceTree) []TopologyEdge {
	var edges []TopologyEdge
	for lbID, fields := range tree[utils.StackResTypeLoadBalancer] {
		lbNodeID := utils.StackResTypeLoadBalancer + "|" + lbID
		refs := findAllRefs(fields, utils.StackResTypeSecurityGroup)
		for _, sgID := range refs {
			edges = append(edges, TopologyEdge{
				From: lbNodeID,
				To:   utils.StackResTypeSecurityGroup + "|" + sgID,
			})
		}
	}
	return edges
}

// findAllRefs searches all field values (including recursing into arrays and
// nested maps) for $ref strings pointing to the given resource type.
func findAllRefs(fields map[string]any, targetType string) []string {
	var refs []string
	seen := make(map[string]bool)
	for key, val := range fields {
		// Check flat keys that end in ".$ref".
		if strings.HasSuffix(key, ".$ref") {
			if id := parseRef(val, targetType); id != "" && !seen[id] {
				seen[id] = true
				refs = append(refs, id)
			}
			continue
		}
		// Recursively search array/map values for nested $ref objects.
		collectRefs(val, targetType, seen, &refs)
	}
	sort.Strings(refs)
	return refs
}

// collectRefs recursively walks a value looking for map entries with a "$ref"
// key that points to the target resource type.
func collectRefs(val any, targetType string, seen map[string]bool, refs *[]string) {
	switch v := val.(type) {
	case map[string]any:
		if refVal, ok := v["$ref"]; ok {
			if id := parseRef(refVal, targetType); id != "" && !seen[id] {
				seen[id] = true
				*refs = append(*refs, id)
			}
		}
		for _, child := range v {
			collectRefs(child, targetType, seen, refs)
		}
	case []any:
		for _, item := range v {
			collectRefs(item, targetType, seen, refs)
		}
	}
}

// parseRef extracts the resource ID from a $ref value like:
// "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns/my-tg:80/status/targetGroupARN"
// It validates that the referenced type matches expectedType.
func parseRef(val any, expectedType string) string {
	s, ok := val.(string)
	if !ok || !strings.HasPrefix(s, "#/resources/") {
		return ""
	}
	// Strip "#/resources/" prefix.
	rest := strings.TrimPrefix(s, "#/resources/")

	// The type contains "::" which is a reliable separator.
	// Format: <Type>/<ResourceID>/status/<field>
	// Type is "AWS::...::Thing" — find the end of the type by matching expectedType.
	if !strings.HasPrefix(rest, expectedType+"/") {
		return ""
	}
	rest = strings.TrimPrefix(rest, expectedType+"/")

	// The remaining part is "<resourceID>/status/<field>".
	// resourceID can contain slashes (e.g., "ns/name:port"), so we find "/status/" from the end.
	idx := strings.LastIndex(rest, "/status/")
	if idx < 0 {
		return ""
	}
	return rest[:idx]
}
