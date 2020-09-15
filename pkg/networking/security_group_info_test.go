package networking

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_buildIPPermissionLabelsForDescription(t *testing.T) {
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
			got := buildIPPermissionLabelsForDescription(tt.args.description)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildIPPermissionDescriptionForLabels(t *testing.T) {
	type args struct {
		labels map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "one label",
			args: args{
				labels: map[string]string{
					"key1": "value1",
				},
			},
			want: "key1=value1",
		},
		{
			name: "two label",
			args: args{
				labels: map[string]string{
					"elbv2.k8s.aws/targetGroupBinding": "shared",
					"key1":                             "value1",
				},
			},
			want: "elbv2.k8s.aws/targetGroupBinding=shared,key1=value1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildIPPermissionDescriptionForLabels(tt.args.labels)
			assert.Equal(t, tt.want, got)
		})
	}
}
