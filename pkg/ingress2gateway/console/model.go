package console

import (
	"encoding/json"
	"fmt"
)

// stackJSON mirrors the StackSchema produced by the controller's stack marshaller.
// Structure: {"id": "ns/name", "resources": {"AWS::...::Type": {"resourceId": {...}}}}
type stackJSON struct {
	ID        string                                `json:"id"`
	Resources map[string]map[string]json.RawMessage `json:"resources"`
}

// ResourceTree is a normalized view of a stack.
// It is needed for field by field comparison.
// Keys: resourceType → resourceID → flattened field path → value.
type ResourceTree map[string]map[string]map[string]any

// ParseStack parses the raw stack JSON annotation into a ResourceTree.
func ParseStack(raw string) (ResourceTree, error) {
	if raw == "" {
		return nil, fmt.Errorf("empty stack JSON")
	}
	var s stackJSON
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, fmt.Errorf("invalid stack JSON: %w", err)
	}
	tree := make(ResourceTree)
	for resType, resources := range s.Resources {
		tree[resType] = make(map[string]map[string]any)
		for resID, rawRes := range resources {
			var resObj map[string]any
			if err := json.Unmarshal(rawRes, &resObj); err != nil {
				return nil, fmt.Errorf("failed to parse resource %s/%s: %w", resType, resID, err)
			}
			flat := make(map[string]any)
			flattenMap("", resObj, flat)
			tree[resType][resID] = flat
		}
	}
	return tree, nil
}

// flattenMap recursively flattens a nested map into dot-separated keys.
func flattenMap(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			flattenMap(key, val, out)
		default:
			out[key] = val
		}
	}
}
