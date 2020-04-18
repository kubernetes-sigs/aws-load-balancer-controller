package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type structA struct {
	Name string
}

type structB struct {
	Name string
	A    *structA
}

func TestDeepCopyInto(t *testing.T) {
	obj := structB{
		Name: "parent",
		A: &structA{
			Name: "child-1",
		},
	}
	objClone := structB{}
	DeepCopyInto(&objClone, obj)
	obj.A.Name = "child-2"

	assert.Equal(t, structB{
		Name: "parent",
		A: &structA{
			Name: "child-2",
		},
	}, obj)
	assert.Equal(t, structB{
		Name: "parent",
		A: &structA{
			Name: "child-1",
		},
	}, objClone)
}
