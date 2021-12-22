package ingress

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	"testing"
)

func Test_defaultModelBuildTask_sortIngressPath(t *testing.T) {
	type args struct {
		paths []networking.HTTPIngressPath
	}
	tests := []struct {
		name string
		args args
		want []networking.HTTPIngressPath
	}{
		{
			name: "Paths sorted by length descending, no ties",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/aaa/bbb",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/aaa/bbb",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
			},
		},
		{
			name: "Paths length descending, when tie, the path with Exact path type precedes",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
					{
						Path:     "/aaa/bbb",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/aaa/bbb",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
				{
					Path:     "/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
			},
		},
		{
			name: "Paths are the same, the path with Exact path type precedes",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.sortIngressPaths(tt.args.paths)
			assert.Equal(t, got, tt.want)
		})
	}
}

func Test_defaultModelBuildTask_buildPathPatterns(t *testing.T) {
	pathTypeImplementationSpecific := networking.PathTypeImplementationSpecific
	pathTypeExact := networking.PathTypeExact
	pathTypePrefix := networking.PathTypePrefix
	pathTypeUnknown := networking.PathType("unknown")
	type args struct {
		path     string
		pathType *networking.PathType
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/* with empty pathType",
			args: args{
				path:     "/*",
				pathType: nil,
			},
			want: []string{"/*"},
		},
		{
			name: "/* with implementationSpecific pathType",
			args: args{
				path:     "/*",
				pathType: &pathTypeImplementationSpecific,
			},
			want: []string{"/*"},
		},
		{
			name: "/* with exact pathType",
			args: args{
				path:     "/*",
				pathType: &pathTypeExact,
			},
			wantErr: errors.New("exact path shouldn't contain wildcards: /*"),
		},
		{
			name: "/* with prefix pathType",
			args: args{
				path:     "/*",
				pathType: &pathTypePrefix,
			},
			wantErr: errors.New("prefix path shouldn't contain wildcards: /*"),
		},
		{
			name: "/abc/ with empty pathType",
			args: args{
				path:     "/abc/",
				pathType: nil,
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/ with implementationSpecific pathType",
			args: args{
				path:     "/abc/",
				pathType: &pathTypeImplementationSpecific,
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/ with exact pathType",
			args: args{
				path:     "/abc/",
				pathType: &pathTypeExact,
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/ with prefix pathType",
			args: args{
				path:     "/abc/",
				pathType: &pathTypePrefix,
			},
			want: []string{"/abc", "/abc/*"},
		},
		{
			name: "/abc/ with unknown pathType",
			args: args{
				path:     "/abc/",
				pathType: &pathTypeUnknown,
			},
			wantErr: errors.New("unsupported pathType: unknown"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatterns(tt.args.path, tt.args.pathType)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildImplementationSpecificPathPatterns(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/ with implementationSpecific pathType",
			args: args{
				path: "/",
			},
			want: []string{"/"},
		},
		{
			name: "/abc with implementationSpecific pathType",
			args: args{
				path: "/abc",
			},
			want: []string{"/abc"},
		},
		{
			name: "/abc/ with implementationSpecific pathType",
			args: args{
				path: "/abc/",
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/def with implementationSpecific pathType",
			args: args{
				path: "/abc/def",
			},
			want: []string{"/abc/def"},
		},
		{
			name: "/abc/def/ with implementationSpecific pathType",
			args: args{
				path: "/abc/def/",
			},
			want: []string{"/abc/def/"},
		},
		{
			name: "/* with implementationSpecific pathType",
			args: args{
				path: "/*",
			},
			want: []string{"/*"},
		},
		{
			name: "/? with implementationSpecific pathType",
			args: args{
				path: "/?",
			},
			want: []string{"/?"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatternsForImplementationSpecificPathType(tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildPathPatternsForExactPathType(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/ with exact pathType",
			args: args{
				path: "/",
			},
			want: []string{"/"},
		},
		{
			name: "/abc with exact pathType",
			args: args{
				path: "/abc",
			},
			want: []string{"/abc"},
		},
		{
			name: "/abc/ with exact pathType",
			args: args{
				path: "/abc/",
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/def with exact pathType",
			args: args{
				path: "/abc/def",
			},
			want: []string{"/abc/def"},
		},
		{
			name: "/abc/def/ with exact pathType",
			args: args{
				path: "/abc/def/",
			},
			want: []string{"/abc/def/"},
		},
		{
			name: "/* with exact pathType",
			args: args{
				path: "/*",
			},
			wantErr: errors.New("exact path shouldn't contain wildcards: /*"),
		},
		{
			name: "/? with exact pathType",
			args: args{
				path: "/?",
			},
			wantErr: errors.New("exact path shouldn't contain wildcards: /?"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatternsForExactPathType(tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildPathPatternsForPrefixPathType(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/ with prefix pathType",
			args: args{
				path: "/",
			},
			want: []string{"/*"},
		},
		{
			name: "/abc with prefix pathType",
			args: args{
				path: "/abc",
			},
			want: []string{"/abc", "/abc/*"},
		},
		{
			name: "/abc/ with prefix pathType",
			args: args{
				path: "/abc/",
			},
			want: []string{"/abc", "/abc/*"},
		},
		{
			name: "/abc/def with prefix pathType",
			args: args{
				path: "/abc/def",
			},
			want: []string{"/abc/def", "/abc/def/*"},
		},
		{
			name: "/abc/def/ with prefix pathType",
			args: args{
				path: "/abc/def/",
			},
			want: []string{"/abc/def", "/abc/def/*"},
		},
		{
			name: "/* with prefix pathType",
			args: args{
				path: "/*",
			},
			wantErr: errors.New("prefix path shouldn't contain wildcards: /*"),
		},
		{
			name: "/? with prefix pathType",
			args: args{
				path: "/?",
			},
			wantErr: errors.New("prefix path shouldn't contain wildcards: /?"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatternsForPrefixPathType(tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}
