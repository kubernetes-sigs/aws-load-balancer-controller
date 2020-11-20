package testutils

import (
	"github.com/golang/mock/gomock"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewListOptionEquals constructs new goMock matcher for client's ListOption
func NewListOptionEquals(expectedListOption client.ListOption) *listOptionEquals {
	return &listOptionEquals{
		expectedListOption: expectedListOption,
	}
}

type listOptionEquals struct {
	expectedListOption client.ListOption
}

var _ gomock.Matcher = &listOptionEquals{}

func (m *listOptionEquals) Matches(x interface{}) bool {
	actualListOpt, ok := x.(client.ListOption)
	if !ok {
		return false
	}
	optA := client.ListOptions{}
	optB := client.ListOptions{}
	actualListOpt.ApplyToList(&optA)
	m.expectedListOption.ApplyToList(&optB)
	return reflect.DeepEqual(optA, optB)
}

func (m *listOptionEquals) String() string {
	return "list option equals"
}
