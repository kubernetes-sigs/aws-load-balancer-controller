package warnings

import (
	"fmt"
	"io"
	"strings"

	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

// CheckInputResources inspects the InputResources for missing referenced
// resources and writes warnings to the provided writer (typically os.Stderr).
// It also detects external target group references in action annotations
// and warns about group.order usage.
// It returns the total number of warnings emitted.
func CheckInputResources(resources *ingress2gateway.InputResources, w io.Writer) int {
	warningCount := 0

	// Build lookup sets
	serviceNames := buildServiceNameSet(resources)
	ingressClassNames := buildIngressClassNameSet(resources)

	hasExternalTG := false
	hasGroupOrder := false
	for _, ing := range resources.Ingresses {
		warningCount += checkMissingServices(ing, serviceNames, w)
		warningCount += checkMissingIngressClass(ing, ingressClassNames, w)
		if !hasExternalTG {
			hasExternalTG = checkExternalTargetGroups(ing)
		}
		if !hasGroupOrder {
			hasGroupOrder = checkGroupOrder(ing)
		}
	}

	if warningCount > 0 {
		fmt.Fprint(w, utils.TipUseFromClusterMessage)
	}

	if hasExternalTG {
		fmt.Fprintln(w, utils.WarnExternalTargetGroupMessage)
	}

	if hasGroupOrder {
		fmt.Fprintln(w, utils.WarnGroupOrderMessage)
	}

	return warningCount
}

func buildServiceNameSet(resources *ingress2gateway.InputResources) map[string]struct{} {
	set := make(map[string]struct{})
	for _, svc := range resources.Services {
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		set[key] = struct{}{}
	}
	return set
}

func buildIngressClassNameSet(resources *ingress2gateway.InputResources) map[string]struct{} {
	set := make(map[string]struct{})
	for _, ic := range resources.IngressClasses {
		set[ic.Name] = struct{}{}
	}
	return set
}

func checkMissingServices(ing networking.Ingress, serviceNames map[string]struct{}, w io.Writer) int {
	warningCount := 0
	namespace := ing.Namespace

	// Check default backend
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		svcKey := fmt.Sprintf("%s/%s", namespace, ing.Spec.DefaultBackend.Service.Name)
		if _, ok := serviceNames[svcKey]; !ok {
			fmt.Fprintf(w, "WARNING: Ingress %q references Service %q but it was not provided. Service-level annotation overrides may be missing.\n",
				fmt.Sprintf("%s/%s", namespace, ing.Name), ing.Spec.DefaultBackend.Service.Name)
			warningCount++
		}
	}

	// Check rule backends
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			// Skip use-annotation backends — the service name is a key into the
			// actions.* annotation, not a real K8s Service.
			if path.Backend.Service.Port.Name == utils.ServicePortUseAnnotation {
				continue
			}
			svcKey := fmt.Sprintf("%s/%s", namespace, path.Backend.Service.Name)
			if _, ok := serviceNames[svcKey]; !ok {
				fmt.Fprintf(w, "WARNING: Ingress %q references Service %q but it was not provided. Service-level annotation overrides may be missing.\n",
					fmt.Sprintf("%s/%s", namespace, ing.Name), path.Backend.Service.Name)
				warningCount++
			}
		}
	}

	return warningCount
}

func checkMissingIngressClass(ing networking.Ingress, ingressClassNames map[string]struct{}, w io.Writer) int {
	namespace := ing.Namespace

	if ing.Spec.IngressClassName == nil {
		return 0
	}

	className := *ing.Spec.IngressClassName
	if _, ok := ingressClassNames[className]; !ok {
		fmt.Fprintf(w, "WARNING: Ingress %q uses IngressClass %q but it was not provided. IngressClassParams overrides may be missing.\n",
			fmt.Sprintf("%s/%s", namespace, ing.Name), className)
		return 1
	}
	return 0
}

// checkExternalTargetGroups scans action annotations for targetGroupARN or targetGroupName references.
func checkExternalTargetGroups(ing networking.Ingress) bool {
	for key, value := range ing.Annotations {
		if !strings.HasPrefix(key, utils.ActionAnnotationPrefix) {
			continue
		}
		if strings.Contains(value, utils.JSONFieldTargetGroupARN) || strings.Contains(value, utils.JSONFieldTargetGroupName) {
			return true
		}
	}
	return false
}

// checkGroupOrder returns true if the Ingress has a group.order annotation.
func checkGroupOrder(ing networking.Ingress) bool {
	_, ok := ing.Annotations["alb.ingress.kubernetes.io/group.order"]
	return ok
}
