package algorithm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RemoveSliceDuplicates(t *testing.T) {
	type args struct {
		data []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "empty",
			args: args{
				data: []string{},
			},
			want: []string{},
		},
		{
			name: "no duplicate entries",
			args: args{
				data: []string{"a", "b", "c", "d"},
			},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "with duplicates",
			args: args{
				data: []string{"a", "b", "a", "c", "b"},
			},
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveSliceDuplicates(tt.args.data)
			assert.Equal(t, tt.want, got)
		})
	}
}
