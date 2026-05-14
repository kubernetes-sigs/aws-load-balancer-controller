package ingress2gateway

import (
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
