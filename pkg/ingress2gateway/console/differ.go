package console

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
//
// Correlation vs. Raw IDs:
//   - CorrelationID is the join key used to match ingress-side resources
//     against gateway-side resources. For most resource types it equals the
//     raw resource ID from the stack JSON. For TargetGroup /
//     TargetGroupBinding it's derived from the TGB's serviceRef so a renamed
//     resource still aligns.
//   - IngressResourceID and GatewayResourceID hold the raw IDs emitted by
//     each controller. They are what the UI displays in each column so
//     customers still see the exact names the controller will produce.
//
// Known is true when the change is a known, semantic artifact of the
// Ingress→Gateway migration itself (e.g., added migrated-from tag, ALB name
// format, controller default drift). The UI uses this to de-emphasize noise.
type DiffEntry struct {
	ResourceType      string `json:"resourceType"`
	CorrelationID     string `json:"correlationId"`
	IngressResourceID string `json:"ingressResourceId,omitempty"`
	GatewayResourceID string `json:"gatewayResourceId,omitempty"`

	Field      string     `json:"field"`
	Ingress    any        `json:"ingress,omitempty"`
	Gateway    any        `json:"gateway,omitempty"`
	Status     DiffStatus `json:"status"`
	Known      bool       `json:"known,omitempty"`
	KnownCause string     `json:"knownCause,omitempty"`
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
	Entries  []DiffEntry    `json:"entries"`
	Summary  DiffSummary    `json:"summary"`
	Topology TopologyResult `json:"topology"`
}

// correlated bundles a raw ID with the flattened fields it points to so we
// can defer the "ingress-side vs gateway-side" decision until after matching.
type correlated struct {
	rawID  string
	fields map[string]any
}

// Diff compares two ResourceTrees and produces a DiffResult. Resources are
// matched across trees by their correlation ID (see correlate.go) rather
// than by their raw IDs, so a TargetGroup that was renamed during migration
// still shows as a single "changed" resource with field-level deltas.
//
// userSpecified indicates which model fields were explicitly set by the user
// via Ingress annotations.
func Diff(ingress, gateway ResourceTree, userSpecified UserSpecifiedFields) DiffResult {
	var entries []DiffEntry

	// Collect all resource types from both trees.
	allTypes := mergedSortedKeys(ingress, gateway)

	for _, resType := range allTypes {
		ingressByCorr := groupByCorrelation(resType, ingress)
		gatewayByCorr := groupByCorrelation(resType, gateway)

		allCorrs := mergedSortedKeys(ingressByCorr, gatewayByCorr)

		for _, corr := range allCorrs {
			inRes, inOK := ingressByCorr[corr]
			gwRes, gwOK := gatewayByCorr[corr]

			switch {
			case inOK && gwOK:
				entries = append(entries, diffFields(resType, corr, inRes, gwRes)...)
			case inOK:
				for _, field := range sortedKeys(inRes.fields) {
					entries = append(entries, DiffEntry{
						ResourceType:      resType,
						CorrelationID:     corr,
						IngressResourceID: inRes.rawID,
						Field:             field,
						Ingress:           inRes.fields[field],
						Status:            StatusRemoved,
					})
				}
			case gwOK:
				for _, field := range sortedKeys(gwRes.fields) {
					entries = append(entries, DiffEntry{
						ResourceType:      resType,
						CorrelationID:     corr,
						GatewayResourceID: gwRes.rawID,
						Field:             field,
						Gateway:           gwRes.fields[field],
						Status:            StatusAdded,
					})
				}
			}
		}
	}

	summary := DiffSummary{}
	for i := range entries {
		// Classify each entry as a known migration artifact or not.
		c := classifyEntry(entries[i], userSpecified)
		entries[i].Known = c.Known
		entries[i].KnownCause = c.Reason

		switch entries[i].Status {
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

// groupByCorrelation returns the resources of a single type keyed by their
// correlation ID. The correlation is computed against the full per-side
// tree so it can cross-reference (e.g., a TargetGroup looking up its TGB).
func groupByCorrelation(resType string, tree ResourceTree) map[string]correlated {
	out := make(map[string]correlated)
	for rawID, fields := range tree[resType] {
		corr := correlationID(resType, rawID, tree)
		out[corr] = correlated{rawID: rawID, fields: fields}
	}
	return out
}

// diffFields compares two flat field maps for a correlated resource pair.
func diffFields(resType, corr string, ingress, gateway correlated) []DiffEntry {
	var entries []DiffEntry
	allFields := mergedSortedKeys(ingress.fields, gateway.fields)

	for _, field := range allFields {
		inVal, inOK := ingress.fields[field]
		gwVal, gwOK := gateway.fields[field]

		entry := DiffEntry{
			ResourceType:      resType,
			CorrelationID:     corr,
			IngressResourceID: ingress.rawID,
			GatewayResourceID: gateway.rawID,
			Field:             field,
		}

		switch {
		case inOK && gwOK:
			entry.Ingress = inVal
			entry.Gateway = gwVal
			if semanticEqual(inVal, gwVal) {
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

// semanticEqual reports whether two JSON-decoded values represent the same
// content, treating slices as multisets.
func semanticEqual(a, b any) bool {
	return cmp.Equal(a, b, cmpopts.SortSlices(canonicalLess))
}

func canonicalLess(x, y any) bool {
	return canonicalBytes(x) < canonicalBytes(y)
}

func canonicalBytes(v any) string {
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}
