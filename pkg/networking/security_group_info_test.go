package networking

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_buildIPPermissionLabelForDescription(t *testing.T) {
	type args struct {
		description string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "empty description",
			args: args{
				description: "",
			},
			want: map[string]string{
				labelKeyRawDescription: "",
			},
		},
		{
			name: "non-empty description",
			args: args{
				description: "some-description",
			},
			want: map[string]string{
				labelKeyRawDescription: "some-description",
			},
		},
		{
			name: "single k-v pair description",
			args: args{
				description: "key1=value1",
			},
			want: map[string]string{
				labelKeyRawDescription: "key1=value1",
				"key1":                 "value1",
			},
		},
		{
			name: "multiple k-v pair description",
			args: args{
				description: "key1=value1,key2=value2",
			},
			want: map[string]string{
				labelKeyRawDescription: "key1=value1,key2=value2",
				"key1":                 "value1",
				"key2":                 "value2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildIPPermissionLabelForDescription(tt.args.description)
			assert.Equal(t, tt.want, got)
		})
	}
}
