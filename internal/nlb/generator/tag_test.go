package generator

import (
	"testing"

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
		TagKeyNamespace:                 "namespace",
		"key":                           "value",
	}
	assert.Equal(t, gen.TagLB("namespace", "ingress"), expected)
	assert.Equal(t, gen.TagTGGroup("namespace", "ingress"), expected)
}

func Test_TagTG(t *testing.T) {
	gen := TagGenerator{}
	expected := map[string]string{
		TagKeyServiceName: "service",
		TagKeyServicePort: "port",
	}
	assert.Equal(t, gen.TagTG("service", "port"), expected)
}
