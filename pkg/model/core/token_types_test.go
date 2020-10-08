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

func TestResourceFieldStringToken_Resolve(t *testing.T) {
	stack := NewDefaultStack(StackID{Namespace: "namespace", Name: "name"})
	resWithStatus := NewFakeResource(stack, "Fake", "fake", FakeResourceSpec{}, &FakeResourceStatus{FieldB: "value"})
	resWithoutStatus := NewFakeResource(stack, "Fake", "fake", FakeResourceSpec{}, nil)

	tests := []struct {
		name    string
		t       StringToken
		want    string
		wantErr error
	}{
		{
			name: "ResourceFieldStringToken with status fulfilled",
			t:    resWithStatus.FieldB(),
			want: "value",
		},
		{
			name:    "ResourceFieldStringToken without status fulfilled",
			t:       resWithoutStatus.FieldB(),
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
	stack := NewDefaultStack(StackID{Namespace: "namespace", Name: "name"})
	resWithStatus := NewFakeResource(stack, "Fake", "fake", FakeResourceSpec{}, &FakeResourceStatus{FieldB: "value"})
	tests := []struct {
		name string
		t    StringToken
		want []Resource
	}{
		{
			name: "ResourceFieldStringToken have dependency on the resource itself",
			t:    resWithStatus.FieldB(),
			want: []Resource{resWithStatus},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			got := tt.t.Dependencies()
			assert.Equal(t, tt.want, got)
		})
	}
}
