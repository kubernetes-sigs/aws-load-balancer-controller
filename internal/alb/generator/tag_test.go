package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_TagLB(t *testing.T) {
	gen := TagGenerator{
		ClusterName: "cluster",
		DefaultTags: map[string]string{
			"key": "value",
		},
	}
	expected := map[string]string{
		"kubernetes.io/cluster/cluster": "owned",
		TagKeyIngressName:               "ingress",
		TagKeyNamespace:                 "namespace",

		"ingress.k8s.aws/cluster":  "cluster",
		"ingress.k8s.aws/stack":    "namespace/ingress",
		"ingress.k8s.aws/resource": "LoadBalancer",
		"key":                      "value",
	}

	assert.Equal(t, gen.TagLB("namespace", "ingress"), expected)
}

func Test_TagTGGroup(t *testing.T) {
	gen := TagGenerator{
		ClusterName: "cluster",
		DefaultTags: map[string]string{
			"key": "value",
		},
	}
	expected := map[string]string{
		"kubernetes.io/cluster/cluster": "owned",
		TagKeyIngressName:               "ingress",
		TagKeyNamespace:                 "namespace",

		"ingress.k8s.aws/cluster": "cluster",
		"ingress.k8s.aws/stack":   "namespace/ingress",
		"key":                     "value",
	}

	assert.Equal(t, gen.TagTGGroup("namespace", "ingress"), expected)
}

func Test_TagTG(t *testing.T) {
	gen := TagGenerator{}
	expected := map[string]string{
		TagKeyServiceName:          "service",
		TagKeyServicePort:          "port",
		"ingress.k8s.aws/resource": "namespace/ingress-service:port",
	}
	assert.Equal(t, gen.TagTG("namespace", "ingress", "service", "port"), expected)
}
