package console

import (
	"fmt"

	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway/utils"
)

// correlationID returns the key used to match a resource from the ingress
// stack against its counterpart in the gateway stack. For most resources this
// is just the raw ID from the stack JSON. For TargetGroup and
// TargetGroupBinding the controllers generate different raw IDs on each side
// even when they represent the same backing service — so we derive a semantic
// ID from the TargetGroupBinding's serviceRef (the canonical source of truth
// for "which service does this target").
//
// Returns an empty string only when the tree is nil; callers should fall back
// to the raw ID in that case.
func correlationID(resType, rawID string, tree ResourceTree) string {
	if tree == nil {
		return rawID
	}

	switch resType {
	case utils.StackResTypeTargetGroup, utils.StackResTypeTargetGroupBinding:
		// Look up the TGB with the same raw ID. Both TG and TGB use the same key.
		tgbResources := tree[utils.StackResTypeTargetGroupBinding]
		tgb, ok := tgbResources[rawID]
		if !ok {
			return rawID
		}
		svc := getServiceRef(tgb)
		if svc == "" {
			return rawID
		}
		return svc
	}
	return rawID
}

// getServiceRef pulls "<svcName>:<port>" out of a flattened
// TargetGroupBinding field map.
func getServiceRef(fields map[string]any) string {
	name, _ := fields["spec.template.spec.serviceRef.name"].(string)
	if name == "" {
		return ""
	}
	// serviceRef.port can be a number (JSON unmarshals to float64) or,
	// in some generators, a string. Normalize via fmt.
	port := fields["spec.template.spec.serviceRef.port"]
	if port == nil {
		return ""
	}
	return fmt.Sprintf("%s:%v", name, port)
}
