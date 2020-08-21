package core

import (
	"context"
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
