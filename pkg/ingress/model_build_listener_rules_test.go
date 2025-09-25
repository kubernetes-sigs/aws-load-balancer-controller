package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
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
			name: "Exact path only",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
			},
		},
		{
			name: "Prefix path only",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/example",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/tea",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/example",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/tea",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
			},
		},
		{
			name: "ImplementationSpecific path only",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/a",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/test",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/a",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/test",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
		{
			name: "Exact and prefix paths",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
					{
						Path:     "/aaa/bbb",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/abc/abc/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
				{
					Path:     "/abc/abc/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
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
		{
			name: "Prefix and ImplementationSpecific paths",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/b",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/example",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/example",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/b",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/ccc",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
		{
			name: "All three types",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/b",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/example",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/test",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/test",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
				{
					Path:     "/example",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/b",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/ccc",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, _ := task.sortIngressPaths(tt.args.paths)
			assert.Equal(t, got, tt.want)
		})
	}
}

func Test_defaultModelBuildTask_classifyIngressPathsByType(t *testing.T) {
	type args struct {
		paths []networking.HTTPIngressPath
	}
	tests := []struct {
		name                            string
		args                            args
		wantExactPaths                  []networking.HTTPIngressPath
		wantPrefixPaths                 []networking.HTTPIngressPath
		wantImplementationSpecificPaths []networking.HTTPIngressPath
		wantErr                         error
	}{
		{
			name: "Paths contain path with invalid pathType, return error",
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
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("xyz")),
					},
				},
			},
			wantErr: errors.New("unknown pathType for path /ccc"),
		},
		{
			name: "Paths contain all three pathTypes",
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
					{
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
				},
			},
			wantExactPaths: []networking.HTTPIngressPath{
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
			},
			wantPrefixPaths: []networking.HTTPIngressPath{
				{
					Path:     "/aaa/bbb",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
			},
			wantImplementationSpecificPaths: []networking.HTTPIngressPath{
				{
					Path:     "/ccc",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
		{
			name: "only exact path",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			wantExactPaths: []networking.HTTPIngressPath{
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			gotExactPaths, gotPrefixPaths, gotImplementationSpecificPaths, err := task.classifyIngressPathsByType(tt.args.paths)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, gotExactPaths, tt.wantExactPaths)
				assert.Equal(t, gotPrefixPaths, tt.wantPrefixPaths)
				assert.Equal(t, gotImplementationSpecificPaths, tt.wantImplementationSpecificPaths)
			}
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

func Test_defaultModelBuildTask_buildListenerRuleTags(t *testing.T) {
	type fields struct {
		ing         ClassifiedIngress
		defaultTags map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]string
		wantErr error
	}{
		{
			name: "empty default tags, no tags annotation",
			fields: fields{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "awesome-ns",
							Name:        "ing-1",
							Annotations: map[string]string{},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{},
		},
		{
			name: "empty default tags, non-empty tags annotation",
			fields: fields{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2",
							},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non-empty default tags, empty tags annotation",
			fields: fields{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "awesome-ns",
							Name:        "ing-1",
							Annotations: map[string]string{},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "non-empty default tags, non-empty tags annotation",
			fields: fields{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3a",
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
				"k4": "v4",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				defaultTags:      tt.fields.defaultTags,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				featureGates:     config.NewFeatureGates(),
			}
			got, err := task.buildListenerRuleTags(context.Background(), tt.fields.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildListenerRuleTags_FeatureGate(t *testing.T) {
	type fields struct {
		ing                 ClassifiedIngress
		defaultTags         map[string]string
		enabledFeatureGates func() config.FeatureGates
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]string
		wantErr error
	}{
		{
			name: "default tags take priority when feature gate disabled",
			fields: fields{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3",
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k1": "v10",
					"k2": "v20",
				},
				enabledFeatureGates: func() config.FeatureGates {
					featureGates := config.NewFeatureGates()
					featureGates.Disable(config.EnableDefaultTagsLowPriority)
					return featureGates
				},
			},
			want: map[string]string{
				"k1": "v10",
				"k2": "v20",
				"k3": "v3",
			},
		},
		{
			name: "annotation tags take priority when feature gate enabled",
			fields: fields{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3",
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k1": "v10",
					"k2": "v20",
				},
				enabledFeatureGates: func() config.FeatureGates {
					featureGates := config.NewFeatureGates()
					featureGates.Enable(config.EnableDefaultTagsLowPriority)
					return featureGates
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				defaultTags:      tt.fields.defaultTags,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				featureGates:     tt.fields.enabledFeatureGates(),
			}
			got, err := task.buildListenerRuleTags(context.Background(), tt.fields.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			for key, value := range tt.want {
				assert.Contains(t, got, key)
				assert.Equal(t, value, got[key])
			}
		})
	}
}
