package ingress2gateway

import (
	"fmt"
	"maps"

	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ingressGroupOutput holds the built resources ready for creation.
type ingressGroupOutput struct {
	IngressClass *networking.IngressClass
	Ingresses    []*networking.Ingress
}

// buildBasicIngressGroup builds an IngressGroup with 2 members:
//   - member-1: admin.example.com / -> svcC
//   - member-2: app.example.com /api -> svcA, /health -> svcB
//
// Includes health check annotations and user tags.
func buildBasicIngressGroup(namespace, groupName string, ipFamily string, svcAName, svcBName, svcCName string) ingressGroupOutput {
	pathPrefix := networking.PathTypePrefix
	ingressClassName := namespace

	ingClass := &networking.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: ingressClassName,
		},
		Spec: networking.IngressClassSpec{
			Controller: ingressController,
		},
	}

	sharedAnnotations := map[string]string{
		annotationScheme:              "internet-facing",
		annotationTargetType:          "ip",
		annotationGroupName:           groupName,
		annotationTags:                "Team=e2e,Component=migration",
		annotationHealthCheckPath:     pathHealth,
		annotationHealthyThreshold:    "2",
		annotationUnhealthyThreshold:  "3",
		annotationHealthCheckInterval: "15",
	}
	if ipFamily == "IPv6" {
		sharedAnnotations[annotationIPAddressType] = "dualstack"
	}

	ing1Annotations := maps.Clone(sharedAnnotations)
	ing1Annotations[annotationGroupOrder] = "1"

	ing2Annotations := maps.Clone(sharedAnnotations)
	ing2Annotations[annotationGroupOrder] = "2"

	ing1 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        "member-1",
			Annotations: ing1Annotations,
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networking.IngressRule{
				{
					Host: hostAdmin,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{Path: pathRoot, PathType: &pathPrefix, Backend: ingressBackend(svcCName)},
							},
						},
					},
				},
			},
		},
	}

	ing2 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        "member-2",
			Annotations: ing2Annotations,
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networking.IngressRule{
				{
					Host: hostApp,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{Path: pathAPI, PathType: &pathPrefix, Backend: ingressBackend(svcAName)},
								{Path: pathHealth, PathType: &pathPrefix, Backend: ingressBackend(svcBName)},
							},
						},
					},
				},
			},
		},
	}

	return ingressGroupOutput{
		IngressClass: ingClass,
		Ingresses:    []*networking.Ingress{ing1, ing2},
	}
}

func ingressBackend(svcName string) networking.IngressBackend {
	return networking.IngressBackend{
		Service: &networking.IngressServiceBackend{
			Name: svcName,
			Port: networking.ServiceBackendPort{Number: 80},
		},
	}
}

func actionBackend(actionName string) networking.IngressBackend {
	return networking.IngressBackend{
		Service: &networking.IngressServiceBackend{
			Name: actionName,
			Port: networking.ServiceBackendPort{Name: "use-annotation"},
		},
	}
}

// buildComplexSingleIngress builds a single Ingress that exercises many annotation categories:
// - HTTP-only listen-ports, scheme, LB attributes, tags
// - Health check overrides, TG attributes
// - Action annotation (fixed-response) on one path
// - source-ip condition on the action path (restricts which clients hit the action)
// - Paths ordered longest-first to match Gateway API precedence
func buildComplexSingleIngress(namespace, ingressClassName string) *networking.Ingress {
	pathPrefix := networking.PathTypePrefix
	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "complex-single",
			Namespace: namespace,
			Annotations: map[string]string{
				annotationScheme:      "internet-facing",
				annotationTargetType:  "ip",
				annotationListenPorts: `[{"HTTP":80}]`,
				"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=120",
				annotationTags:            "Env=prod,Team=platform",
				annotationHealthCheckPath: "/health",
				"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "10",
				"alb.ingress.kubernetes.io/healthcheck-timeout-seconds":  "3",
				"alb.ingress.kubernetes.io/healthy-threshold-count":      "3",
				"alb.ingress.kubernetes.io/unhealthy-threshold-count":    "2",
				"alb.ingress.kubernetes.io/success-codes":                "200-299",
				"alb.ingress.kubernetes.io/target-group-attributes":      "deregistration_delay.timeout_seconds=30",
				"alb.ingress.kubernetes.io/actions.maintenance":          `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"503","messageBody":"Under maintenance"}}`,
				"alb.ingress.kubernetes.io/conditions.maintenance":       `[{"field":"source-ip","sourceIpConfig":{"values":["10.0.0.0/8","192.168.0.0/16"]}}]`,
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networking.IngressRule{
				{
					Host: hostApp,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{Path: "/api/v1/resources", PathType: &pathPrefix, Backend: ingressBackend("svc-api")},
								{Path: "/api/v1", PathType: &pathPrefix, Backend: ingressBackend("svc-static")},
								{Path: "/maintenance", PathType: &pathPrefix, Backend: actionBackend("maintenance")},
							},
						},
					},
				},
			},
		},
	}
}

// buildGroupMemberA builds the first member of a cross-namespace group:
//   - action: fixed-response on /api/maintenance, weighted forward on /api/forward
//   - Paths are longer than member-b's so Gateway API precedence (path length)
//     matches Ingress group.order precedence (member-a first).
func buildGroupMemberA(namespace, groupName, ingressClassName string) *networking.Ingress {
	pathPrefix := networking.PathTypePrefix
	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "member-a",
			Namespace: namespace,
			Annotations: map[string]string{
				annotationGroupName:   groupName,
				annotationGroupOrder:  "1",
				annotationScheme:      "internal",
				annotationTargetType:  "ip",
				annotationListenPorts: `[{"HTTP":80}]`,
				annotationTags:        "ManagedBy=teamA,Feature=api",
				"alb.ingress.kubernetes.io/actions.maintenance-action": `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"503","messageBody":"Under maintenance"}}`,
				"alb.ingress.kubernetes.io/actions.weighted-forward":   `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc-blue","servicePort":"80","weight":80},{"serviceName":"svc-green","servicePort":"80","weight":20}]}}`,
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networking.IngressRule{
				{
					Host: hostAPI,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{Path: "/api/maintenance", PathType: &pathPrefix, Backend: actionBackend("maintenance-action")},
								{Path: "/api/forward", PathType: &pathPrefix, Backend: actionBackend("weighted-forward")},
							},
						},
					},
				},
			},
		},
	}
}

// buildGroupMemberB builds the second member of a cross-namespace group:
// - listen-ports HTTP:80
// - group.order set (trigger warning)
// - Path is shorter than member-a's paths so Gateway API precedence matches group.order
func buildGroupMemberB(namespace, groupName, ingressClassName string) *networking.Ingress {
	pathPrefix := networking.PathTypePrefix
	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "member-b",
			Namespace: namespace,
			Annotations: map[string]string{
				annotationGroupName:   groupName,
				annotationGroupOrder:  "2",
				annotationTargetType:  "ip",
				annotationListenPorts: `[{"HTTP":80}]`,
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networking.IngressRule{
				{
					Host: hostAPI,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{Path: "/search", PathType: &pathPrefix, Backend: ingressBackend("svc-search")},
							},
						},
					},
				},
			},
		},
	}
}

// buildPlatformGroupMember builds a simple platform group member.
// pathSuffix should be longer for member-1 than member-2 to keep priority
// deterministic across both Ingress (group.order) and Gateway (path length) controllers.
func buildPlatformGroupMember(namespace, groupName, ingressClassName, svcName, order, pathSuffix string) *networking.Ingress {
	pathPrefix := networking.PathTypePrefix
	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("platform-%s", svcName),
			Namespace: namespace,
			Annotations: map[string]string{
				annotationScheme:     "internet-facing",
				annotationTargetType: "ip",
				annotationGroupName:  groupName,
				annotationGroupOrder: order,
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networking.IngressRule{{
				Host: fmt.Sprintf("%s.platform.example.com", svcName),
				IngressRuleValue: networking.IngressRuleValue{
					HTTP: &networking.HTTPIngressRuleValue{
						Paths: []networking.HTTPIngressPath{
							{Path: pathSuffix, PathType: &pathPrefix, Backend: ingressBackend(svcName)},
						},
					},
				},
			}},
		},
	}
}

// buildStandaloneIngress builds an ungrouped Ingress.
func buildStandaloneIngress(namespace, ingressClassName, svcName string) *networking.Ingress {
	pathPrefix := networking.PathTypePrefix
	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone",
			Namespace: namespace,
			Annotations: map[string]string{
				annotationScheme:     "internet-facing",
				annotationTargetType: "ip",
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networking.IngressRule{{
				Host: "standalone.example.com",
				IngressRuleValue: networking.IngressRuleValue{
					HTTP: &networking.HTTPIngressRuleValue{
						Paths: []networking.HTTPIngressPath{
							{Path: "/", PathType: &pathPrefix, Backend: ingressBackend(svcName)},
						},
					},
				},
			}},
		},
	}
}
