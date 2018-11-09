package generator

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/stretchr/testify/assert"
)

func Test_TagIngress(t *testing.T) {
	gen := TagGenerator{
		ClusterName: "cluster",
		DefaultTags: map[string]string{
			"key": "value",
		},
	}
	expected := map[string]string{
		"kubernetes.io/cluster/cluster": "owned",
		tags.IngressName:                "ingress",
		tags.Namespace:                  "namespace",
		"key":                           "value",
	}
	assert.Equal(t, gen.TagLB("namespace", "ingress"), expected)
	assert.Equal(t, gen.TagTGGroup("namespace", "ingress"), expected)
}

func Test_TagTG(t *testing.T) {
	gen := TagGenerator{}
	expected := map[string]string{
		tags.ServiceName: "service",
		tags.ServicePort: "port",
	}
	assert.Equal(t, gen.TagTG("service", "port"), expected)
}
