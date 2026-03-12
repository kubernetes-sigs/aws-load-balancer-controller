package warnings

import (
	"fmt"
	"io"

	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
)

// CheckMissingResources inspects the InputResources for missing referenced
// resources and writes warnings to the provided writer (typically os.Stderr).
// It returns the total number of warnings emitted.
func CheckMissingResources(resources *ingress2gateway.InputResources, w io.Writer) int {
	warningCount := 0

	// Build lookup sets
	serviceNames := buildServiceNameSet(resources)
	ingressClassNames := buildIngressClassNameSet(resources)

	for _, ing := range resources.Ingresses {
		// Check for missing Services referenced in backends
		warningCount += checkMissingServices(ing, serviceNames, w)

		// Check for missing IngressClass
		warningCount += checkMissingIngressClass(ing, ingressClassNames, w)
	}

	if warningCount > 0 {
		fmt.Fprintf(w, "\nTip: Use --from-cluster to automatically read all referenced resources,\n")
		fmt.Fprintf(w, "     or include Service/IngressClass/IngressClassParams files in your input.\n\n")
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
	if namespace == "" {
		namespace = "default"
	}

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
	if namespace == "" {
		namespace = "default"
	}

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
