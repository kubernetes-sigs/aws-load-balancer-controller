package runtime

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_conciseError_Error(t *testing.T) {
	type fields struct {
		err error
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "plain Error",
			fields: fields{
				err: errors.New("plain error"),
			},
			want: "plain error",
		},
		{
			name: "wrapped Error",
			fields: fields{
				err: errors.Wrap(errors.New("plain error"), "wrapped msg"),
			},
			want: "wrapped msg: plain error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &conciseError{
				err: tt.fields.err,
			}
			got := e.Error()
			assert.Equal(t, tt.want, got)
		})
	}
}
