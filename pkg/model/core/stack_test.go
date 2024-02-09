package core

import (
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

func Test_defaultStack_StackID(t *testing.T) {
	tests := []struct {
		name  string
		stack Stack
		want  StackID
	}{
		{
			name:  "stack with ID",
			stack: NewDefaultStack(StackID{Namespace: "namespace", Name: "name"}),
			want:  StackID{Namespace: "namespace", Name: "name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stack.StackID()
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultStack_ListResources(t *testing.T) {
	tests := []struct {
		name              string
		operations        func(stack Stack)
		wantFakeResources []*FakeResource
		wantErr           error
	}{
		{
			name: "no fake resources",
			operations: func(stack Stack) {

			},
			wantFakeResources: []*FakeResource{},
			wantErr:           nil,
		},
		{
			name: "one fake resources",
			operations: func(stack Stack) {
				NewFakeResource(stack, "fake", "id-A", FakeResourceSpec{}, nil)
			},
			wantFakeResources: []*FakeResource{
				{
					ResourceMeta: ResourceMeta{
						resType: "fake",
						id:      "id-A",
					},
					Spec:   FakeResourceSpec{},
					Status: nil,
				},
			},
			wantErr: nil,
		},
		{
			name: "multiple fake resources",
			operations: func(stack Stack) {
				NewFakeResource(stack, "fake", "id-A", FakeResourceSpec{}, nil)
				NewFakeResource(stack, "fake", "id-B", FakeResourceSpec{}, nil)
			},
			wantFakeResources: []*FakeResource{
				{
					ResourceMeta: ResourceMeta{
						resType: "fake",
						id:      "id-A",
					},
					Spec:   FakeResourceSpec{},
					Status: nil,
				},
				{
					ResourceMeta: ResourceMeta{
						resType: "fake",
						id:      "id-B",
					},
					Spec:   FakeResourceSpec{},
					Status: nil,
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDefaultStack(StackID{Namespace: "namespace", Name: "name"})
			tt.operations(s)
			var gotFakeResources []*FakeResource
			err := s.ListResources(&gotFakeResources)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				for _, want := range tt.wantFakeResources {
					want.ResourceMeta.stack = s
				}
				sort.Slice(gotFakeResources, func(i, j int) bool {
					return gotFakeResources[i].ID() < gotFakeResources[j].ID()
				})
				assert.Equal(t, tt.wantFakeResources, gotFakeResources)
			}
		})
	}
}
