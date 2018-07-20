package tg

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
)

func TestMergeAnnotations(t *testing.T) {
	var tests = []struct {
		ingressAnnotations map[string]string
		serviceAnnotations map[string]string
		expected           annotations.Annotations
		pass               bool
	}{
		{
			map[string]string{
				"alb.ingress.kubernetes.io/subnets":          "subnet-abcdfg",
				"alb.ingress.kubernetes.io/scheme":           "internal",
				"alb.ingress.kubernetes.io/healthcheck-path": "/ingressPath",
			},
			map[string]string{
				"alb.ingress.kubernetes.io/healthcheck-path": "/servicePath",
			},
			annotations.Annotations{
				HealthcheckPath: aws.String("/servicePath"),
			},
			true,
		},
		{
			map[string]string{
				"alb.ingress.kubernetes.io/subnets":          "subnet-abcdfg",
				"alb.ingress.kubernetes.io/scheme":           "internal",
				"alb.ingress.kubernetes.io/healthcheck-path": "/ingressPath",
			},
			map[string]string{},
			annotations.Annotations{
				HealthcheckPath: aws.String("/ingressPath"),
			},
			true,
		},
		{
			map[string]string{
				"alb.ingress.kubernetes.io/subnets":          "subnet-abcdfg",
				"alb.ingress.kubernetes.io/scheme":           "internal",
				"alb.ingress.kubernetes.io/healthcheck-path": "/ingressPath",
			},
			map[string]string{},
			annotations.Annotations{
				HealthcheckPath: aws.String("/"),
			},
			false,
		},
	}
	vf := annotations.NewValidatingAnnotationFactory(&annotations.NewValidatingAnnotationFactoryOptions{
		Validator:   annotations.FakeValidator{VpcId: "vpc-1"},
		ClusterName: aws.String("clusterName")})

	for i, tt := range tests {
		a, err := mergeAnnotations(&mergeAnnotationsOptions{
			AnnotationFactory:  vf,
			IngressAnnotations: &tt.ingressAnnotations,
		})

		if err != nil && tt.pass {
			t.Errorf("mergeAnnotations(%v): got %v expected %v, errored: %v", i, *a.HealthcheckPath, *tt.expected.HealthcheckPath, err)
		}

		if err == nil && tt.pass && *tt.expected.HealthcheckPath != *a.HealthcheckPath {
			t.Errorf("mergeAnnotations(%v): expected %v, actual %v", i, *tt.expected.HealthcheckPath, *a.HealthcheckPath)
		}

		if err == nil && !tt.pass && *tt.expected.HealthcheckPath == *a.HealthcheckPath {
			t.Errorf("mergeAnnotations(%v): not expected %v, actual %v", i, *tt.expected.HealthcheckPath, *a.HealthcheckPath)
		}
	}
}
