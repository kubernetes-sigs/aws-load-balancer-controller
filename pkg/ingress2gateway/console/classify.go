package console

import (
	"regexp"
	"strings"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

// classification describes why a diff entry is considered an expected migration artifact.
type classification struct {
	Expected bool
	Reason   string
}

// migratedFromTagField is the stack-JSON path to the migration tag the
// translator writes onto every generated resource. Stack-JSON flattens
// spec.tags.<key> into one dotted key, so we compose the shared tag key
// rather than duplicate the string.
var migratedFromTagField = "spec.tags." + utils.MigrationTagKey

// albNameRegex matches the controller's auto-generated ALB/TargetGroup/SG name
// format. Two variants are produced by the ingress and gateway controllers:
//   - explicit-group LBs use 2 sections: `k8s-<groupName>-<10 hex>`
//   - standalone / implicit-group LBs and all SGs/TGs use 3 sections:
//     `k8s-<ns>-<name>-<10 hex>`
//
// We accept both so a migration that shifts an explicit group into a per-gateway
// name (the gateway controller always emits the 3-section form) is classified
// as expected when both sides look controller-generated.
var albNameRegex = regexp.MustCompile(`^k8s-[a-z0-9]+(-[a-z0-9]+)?-[0-9a-f]{10}$`)

// healthCheckDefaultFields are controller-default fields that drift between
// the Ingress controller (defaults 2) and Gateway controller (defaults 3).
var healthCheckDefaultFields = map[string]string{
	"spec.healthCheckConfig.healthyThresholdCount":   "Controller default differs (Ingress=2, Gateway=3)",
	"spec.healthCheckConfig.unhealthyThresholdCount": "Controller default differs (Ingress=2, Gateway=3)",
	"spec.healthCheckConfig.matcher.httpCode":        "Controller default differs (Ingress=200, Gateway=200-399)",
}

// UserSpecifiedFields tracks which model fields were explicitly set by the user
// via Ingress annotations (as opposed to being controller defaults).
type UserSpecifiedFields map[string]bool

// classifyEntry returns whether a diff entry is an expected migration artifact.
// Rules are intentionally conservative: we only mark entries that match a known
// pattern so genuine user-visible changes never get hidden.
func classifyEntry(e DiffEntry, userSpecified UserSpecifiedFields) classification {
	// Added migrated-from tag on any resource.
	if e.Status == StatusAdded && e.Field == migratedFromTagField {
		return classification{Expected: true, Reason: "Added by migration tool"}
	}

	// ALB-family resources: name change with controller-generated format on both sides.
	if e.Status == StatusChanged {
		switch {
		case e.Field == "spec.name" && (e.ResourceType == utils.StackResTypeLoadBalancer || e.ResourceType == utils.StackResTypeTargetGroup):
			if matchesALBName(e.Ingress) && matchesALBName(e.Gateway) {
				return classification{Expected: true, Reason: "Controller-generated name; format preserved"}
			}
		case e.Field == "spec.groupName" && e.ResourceType == utils.StackResTypeSecurityGroup:
			if matchesALBName(e.Ingress) && matchesALBName(e.Gateway) {
				return classification{Expected: true, Reason: "Controller-generated name; format preserved"}
			}
		case e.Field == "spec.template.metadata.name" && e.ResourceType == utils.StackResTypeTargetGroupBinding:
			if matchesALBName(e.Ingress) && matchesALBName(e.Gateway) {
				return classification{Expected: true, Reason: "Controller-generated name; format preserved"}
			}
		}
	}

	// Health check defaults drift on TargetGroup — only when the user did NOT
	// explicitly set the corresponding annotation on the Ingress.
	if e.Status == StatusChanged && e.ResourceType == utils.StackResTypeTargetGroup {
		if reason, ok := healthCheckDefaultFields[e.Field]; ok {
			if !userSpecified[e.Field] {
				return classification{Expected: true, Reason: reason}
			}
		}
	}

	// Weight added on ListenerRule forward actions — gateway controller emits
	// weight on every target group; ingress controller omits it.
	if e.Status == StatusAdded && e.ResourceType == utils.StackResTypeListenerRule {
		if strings.Contains(e.Field, "forwardConfig.targetGroups") && strings.HasSuffix(e.Field, ".weight") {
			return classification{Expected: true, Reason: "Gateway API always sets forward weight"}
		}
	}

	// ListenerRule actions and TargetGroupBindings reference TargetGroups by
	// raw stack ID via targetGroupARN.$ref. The controllers generate different
	// raw IDs for TGs on each side even when the backing service is identical,
	// so a $ref string diff here is an artifact of naming — the underlying
	// endpoint is covered by the TargetGroup row which is correlated by
	// serviceRef.
	if e.Status == StatusChanged {
		switch {
		case e.ResourceType == utils.StackResTypeListenerRule &&
			strings.Contains(e.Field, "forwardConfig.targetGroups") &&
			strings.HasSuffix(e.Field, ".targetGroupARN.$ref"):
			return classification{Expected: true, Reason: "Points at the correlated TargetGroup; see that row for real field diffs"}
		case e.ResourceType == utils.StackResTypeTargetGroupBinding &&
			e.Field == "spec.template.spec.targetGroupARN.$ref":
			return classification{Expected: true, Reason: "Points at the correlated TargetGroup; see that row for real field diffs"}
		}
	}

	return classification{}
}

// annotationToFieldPath maps Ingress annotation suffixes to the model field
// paths they control. Used to determine which health-check fields were
// explicitly set by the user vs. left as controller defaults.
var annotationToFieldPath = map[string]string{
	annotations.IngressSuffixHealthyThresholdCount:   "spec.healthCheckConfig.healthyThresholdCount",
	annotations.IngressSuffixUnhealthyThresholdCount: "spec.healthCheckConfig.unhealthyThresholdCount",
	annotations.IngressSuffixSuccessCodes:            "spec.healthCheckConfig.matcher.httpCode",
}

// buildUserSpecifiedFields scans Ingress annotations and returns the set of
// model field paths that were explicitly configured by the user.
func buildUserSpecifiedFields(ingressAnnotations map[string]string) UserSpecifiedFields {
	usf := make(UserSpecifiedFields)
	for suffix, fieldPath := range annotationToFieldPath {
		key := annotations.AnnotationPrefixIngress + "/" + suffix
		if _, ok := ingressAnnotations[key]; ok {
			usf[fieldPath] = true
		}
	}
	return usf
}

// matchesALBName reports whether v is a string matching the controller's
// generated ALB name format `k8s-<ns>-<base>-<10 hex>`.
func matchesALBName(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	return albNameRegex.MatchString(s)
}
