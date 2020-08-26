package core

import (
	"context"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLiteralStringToken_Resolve(t *testing.T) {
	tests := []struct {
		name    string
		t       LiteralStringToken
		want    string
		wantErr error
	}{
		{
			name: "empty string",
			t:    LiteralStringToken(""),
			want: "",
		},
		{
			name: "non-empty string",
			t:    LiteralStringToken("my-token"),
			want: "my-token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.t.Resolve(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestLiteralStringToken_Dependencies(t *testing.T) {
	tests := []struct {
		name string
		t    LiteralStringToken
		want []Resource
	}{
		{
			name: "LiteralStringToken have no dependency",
			t:    LiteralStringToken("my-token"),
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.t.Dependencies()
			assert.Equal(t, tt.want, got)
		})
	}
}

var _ Resource = &FakeResource{}

type FakeResource struct {
	status *FakeResourceStatus `json:"status,omitempty"`
}

type FakeResourceStatus struct {
	Field string `json:"field"`
}

func (r *FakeResource) ID() string {
	return "fake"
}

func TestResourceFieldStringToken_Resolve(t *testing.T) {
	resWithStatus := &FakeResource{
		status: &FakeResourceStatus{Field: "value"},
	}
	resWithoutStatus := &FakeResource{
		status: nil,
	}

	tests := []struct {
		name    string
		t       *ResourceFieldStringToken
		want    string
		wantErr error
	}{
		{
			name: "ResourceFieldStringToken with status fulfilled",
			t: NewResourceFieldStringToken(resWithStatus, "status/field",
				func(ctx context.Context, res Resource, fieldPath string) (s string, err error) {
					fr := res.(*FakeResource)
					if fr.status == nil {
						return "", errors.Errorf("FakeResource is not fulfilled yet: %v", fr.ID())
					}
					return fr.status.Field, nil
				},
			),
			want: "value",
		},
		{
			name: "ResourceFieldStringToken without status fulfilled",
			t: NewResourceFieldStringToken(resWithoutStatus, "status/field",
				func(ctx context.Context, res Resource, fieldPath string) (s string, err error) {
					fr := res.(*FakeResource)
					if fr.status == nil {
						return "", errors.Errorf("FakeResource is not fulfilled yet: %v", fr.ID())
					}
					return fr.status.Field, nil
				},
			),
			wantErr: errors.New("FakeResource is not fulfilled yet: fake"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			got, err := tt.t.Resolve(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestResourceFieldStringToken_Dependencies(t *testing.T) {
	res := &FakeResource{}
	tests := []struct {
		name string
		t    *ResourceFieldStringToken
		want []Resource
	}{
		{
			name: "ResourceFieldStringToken have dependency on the resource itself",
			t: NewResourceFieldStringToken(res, "status/field",
				func(ctx context.Context, res Resource, fieldPath string) (s string, err error) {
					fr := res.(*FakeResource)
					if fr.status == nil {
						return "", errors.Errorf("FakeResource is not fulfilled yet: %v", fr.ID())
					}
					return fr.status.Field, nil
				},
			),
			want: []Resource{res},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			got := tt.t.Dependencies()
			assert.Equal(t, tt.want, got)
		})
	}
}
