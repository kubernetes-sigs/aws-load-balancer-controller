package console

import (
	"fmt"
	"sort"
)

// DiffStatus represents the comparison status of a field or resource.
type DiffStatus string

const (
	StatusSame    DiffStatus = "same"
	StatusChanged DiffStatus = "changed"
	StatusAdded   DiffStatus = "added"
	StatusRemoved DiffStatus = "removed"
)

// DiffEntry represents a single field-level difference between two models.
type DiffEntry struct {
	ResourceType string     `json:"resourceType"`
	ResourceID   string     `json:"resourceId"`
	Field        string     `json:"field"`
	Ingress      any        `json:"ingress,omitempty"`
	Gateway      any        `json:"gateway,omitempty"`
	Status       DiffStatus `json:"status"`
}

// DiffSummary counts entries by status.
// This is only a quick count summary, details are in Entries.
type DiffSummary struct {
	Same    int `json:"same"`
	Changed int `json:"changed"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// DiffResult holds the complete comparison between an ingress and gateway model.
type DiffResult struct {
	IngressSource string      `json:"ingressSource"`
	GatewaySource string      `json:"gatewaySource"`
	Entries       []DiffEntry `json:"entries"`
	Summary       DiffSummary `json:"summary"`
}

// Diff compares two ResourceTrees and produces a DiffResult.
func Diff(ingress, gateway ResourceTree) DiffResult {
	var entries []DiffEntry

	// Collect all resource types from both trees.
	allTypes := mergedSortedKeys(ingress, gateway)

	for _, resType := range allTypes {
		ingressResources := ingress[resType]
		gatewayResources := gateway[resType]

		allIDs := mergedSortedKeys(ingressResources, gatewayResources)

		for _, resID := range allIDs {
			inFields, inOK := ingressResources[resID]
			gwFields, gwOK := gatewayResources[resID]

			if inOK && gwOK {
				// Resource exists in both — diff fields.
				entries = append(entries, diffFields(resType, resID, inFields, gwFields)...)
			} else if inOK {
				// Resource only in ingress — removed.
				for _, field := range sortedKeys(inFields) {
					entries = append(entries, DiffEntry{
						ResourceType: resType,
						ResourceID:   resID,
						Field:        field,
						Ingress:      inFields[field],
						Status:       StatusRemoved,
					})
				}
			} else {
				// Resource only in gateway — added.
				for _, field := range sortedKeys(gwFields) {
					entries = append(entries, DiffEntry{
						ResourceType: resType,
						ResourceID:   resID,
						Field:        field,
						Gateway:      gwFields[field],
						Status:       StatusAdded,
					})
				}
			}
		}
	}

	summary := DiffSummary{}
	for _, e := range entries {
		switch e.Status {
		case StatusSame:
			summary.Same++
		case StatusChanged:
			summary.Changed++
		case StatusAdded:
			summary.Added++
		case StatusRemoved:
			summary.Removed++
		}
	}

	return DiffResult{
		Entries: entries,
		Summary: summary,
	}
}

// diffFields compares two flat field maps for the same resource.
func diffFields(resType, resID string, ingressFields, gatewayFields map[string]any) []DiffEntry {
	var entries []DiffEntry
	allFields := mergedSortedKeys(ingressFields, gatewayFields)

	for _, field := range allFields {
		inVal, inOK := ingressFields[field]
		gwVal, gwOK := gatewayFields[field]

		entry := DiffEntry{
			ResourceType: resType,
			ResourceID:   resID,
			Field:        field,
		}

		switch {
		case inOK && gwOK:
			entry.Ingress = inVal
			entry.Gateway = gwVal
			if fmt.Sprintf("%v", inVal) == fmt.Sprintf("%v", gwVal) {
				entry.Status = StatusSame
			} else {
				entry.Status = StatusChanged
			}
		case inOK:
			entry.Ingress = inVal
			entry.Status = StatusRemoved
		case gwOK:
			entry.Gateway = gwVal
			entry.Status = StatusAdded
		}

		entries = append(entries, entry)
	}
	return entries
}

// mergedSortedKeys collects keys from two maps and returns them sorted.
func mergedSortedKeys[V any](a, b map[string]V) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
