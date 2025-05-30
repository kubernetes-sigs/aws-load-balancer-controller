package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

var MockIngress = ClassifiedIngress{
	Ing: &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "namespace",
			Name:            "ingress-a",
			Annotations:     map[string]string{},
			ResourceVersion: "0001",
		},
	},
}

var MockIngressWithUseRegexPathMatch = ClassifiedIngress{
	Ing: &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "ingress-a",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/use-regex-path-match": "true",
			},
			ResourceVersion: "0001",
		},
	},
}

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
			task := &defaultModelBuildTask{
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
			}
			got, _, err := task.buildPathPatterns(MockIngress, tt.args.path, tt.args.pathType)
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
		ing  ClassifiedIngress
		path string
	}
	tests := []struct {
		name            string
		args            args
		wantValues      []string
		wantRegexValues []string
		wantErr         error
	}{
		{
			name: "/ with implementationSpecific Values pathType",
			args: args{
				ing:  MockIngress,
				path: "/",
			},
			wantValues:      []string{"/"},
			wantRegexValues: []string{},
		},
		{
			name: "/abc with implementationSpecific Values pathType",
			args: args{
				ing:  MockIngress,
				path: "/abc",
			},
			wantValues:      []string{"/abc"},
			wantRegexValues: []string{},
		},
		{
			name: "/abc/ with implementationSpecific Values pathType",
			args: args{
				ing:  MockIngress,
				path: "/abc/",
			},
			wantValues:      []string{"/abc/"},
			wantRegexValues: []string{},
		},
		{
			name: "/abc/def with implementationSpecific Values pathType",
			args: args{
				ing:  MockIngress,
				path: "/abc/def",
			},
			wantValues:      []string{"/abc/def"},
			wantRegexValues: []string{},
		},
		{
			name: "/abc/def/ with implementationSpecific Values pathType",
			args: args{
				ing:  MockIngress,
				path: "/abc/def/",
			},
			wantValues:      []string{"/abc/def/"},
			wantRegexValues: []string{},
		},
		{
			name: "/* with implementationSpecific Values pathType",
			args: args{
				ing:  MockIngress,
				path: "/*",
			},
			wantValues:      []string{"/*"},
			wantRegexValues: []string{},
		},
		{
			name: "/? with implementationSpecific Values pathType",
			args: args{
				ing:  MockIngress,
				path: "/?",
			},
			wantValues:      []string{"/?"},
			wantRegexValues: []string{},
		},

		{
			name: `^/api/.+$ with implementationSpecific RegexValues pathType`,
			args: args{
				ing:  MockIngressWithUseRegexPathMatch,
				path: `/^/api/.+$`,
			},
			wantValues:      []string{},
			wantRegexValues: []string{`^/api/.+$`},
		},
		{
			name: `Invalid use-regex-path-match value`,
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/use-regex-path-match": "foo",
							},
							ResourceVersion: "0001",
						},
					},
				},
				path: "/abc/*",
			},
			wantValues:      []string{"/abc/*"},
			wantRegexValues: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
			}
			gotValues, gotRegexValues, err := task.buildPathPatternsForImplementationSpecificPathType(tt.args.ing, tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, gotValues, tt.wantValues)
				assert.Equal(t, gotRegexValues, tt.wantRegexValues)
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
			got, _, err := task.buildPathPatternsForExactPathType(tt.args.path)
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
			got, _, err := task.buildPathPatternsForPrefixPathType(tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildTransforms(t *testing.T) {
	type args struct {
		backend EnhancedBackend
	}
	tests := []struct {
		name    string
		args    args
		want    []elbv2model.Transform
		wantErr error
	}{
		{
			name: "empty transforms",
			args: args{
				backend: EnhancedBackend{
					Transforms: []Transform{},
				},
			},
			want: []elbv2model.Transform{},
		},
		{
			name: "url rewrite transform",
			args: args{
				backend: EnhancedBackend{
					Transforms: []Transform{
						{
							Type: TransformTypeUrlRewrite,
							UrlRewriteConfig: &RewriteConfigObject{
								Rewrites: []RewriteConfig{
									{
										Regex:   "/path1/(.*)",
										Replace: "/newpath1/$1",
									},
								},
							},
						},
					},
				},
			},
			want: []elbv2model.Transform{
				{
					Type: elbv2model.TransformTypeUrlRewrite,
					UrlRewriteConfig: &elbv2model.RewriteConfigObject{
						Rewrites: []elbv2model.RewriteConfig{
							{
								Regex:   "/path1/(.*)",
								Replace: "/newpath1/$1",
							},
						},
					},
				},
			},
		},
		{
			name: "host header rewrite transform",
			args: args{
				backend: EnhancedBackend{
					Transforms: []Transform{
						{
							Type: TransformTypeHostHeaderRewrite,
							HostHeaderRewriteConfig: &RewriteConfigObject{
								Rewrites: []RewriteConfig{
									{
										Regex:   "example.com",
										Replace: "new-example.com",
									},
								},
							},
						},
					},
				},
			},
			want: []elbv2model.Transform{
				{
					Type: elbv2model.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &elbv2model.RewriteConfigObject{
						Rewrites: []elbv2model.RewriteConfig{
							{
								Regex:   "example.com",
								Replace: "new-example.com",
							},
						},
					},
				},
			},
		},
		{
			name: "multiple transforms",
			args: args{
				backend: EnhancedBackend{
					Transforms: []Transform{
						{
							Type: TransformTypeUrlRewrite,
							UrlRewriteConfig: &RewriteConfigObject{
								Rewrites: []RewriteConfig{
									{
										Regex:   "/path1/(.*)",
										Replace: "/newpath1/$1",
									},
								},
							},
						},
						{
							Type: TransformTypeHostHeaderRewrite,
							HostHeaderRewriteConfig: &RewriteConfigObject{
								Rewrites: []RewriteConfig{
									{
										Regex:   "example.com",
										Replace: "new-example.com",
									},
								},
							},
						},
					},
				},
			},
			want: []elbv2model.Transform{
				{
					Type: elbv2model.TransformTypeUrlRewrite,
					UrlRewriteConfig: &elbv2model.RewriteConfigObject{
						Rewrites: []elbv2model.RewriteConfig{
							{
								Regex:   "/path1/(.*)",
								Replace: "/newpath1/$1",
							},
						},
					},
				},
				{
					Type: elbv2model.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &elbv2model.RewriteConfigObject{
						Rewrites: []elbv2model.RewriteConfig{
							{
								Regex:   "example.com",
								Replace: "new-example.com",
							},
						},
					},
				},
			},
		},
		{
			name: "url rewrite transform with missing config",
			args: args{
				backend: EnhancedBackend{
					Transforms: []Transform{
						{
							Type: TransformTypeUrlRewrite,
						},
					},
				},
			},
			wantErr: errors.New("missing urlRewriteConfig"),
		},
		{
			name: "host header rewrite transform with missing config",
			args: args{
				backend: EnhancedBackend{
					Transforms: []Transform{
						{
							Type: TransformTypeHostHeaderRewrite,
						},
					},
				},
			},
			wantErr: errors.New("missing hostHeaderRewriteConfig"),
		},
		{
			name: "unknown transform type",
			args: args{
				backend: EnhancedBackend{
					Transforms: []Transform{
						{
							Type: "unknown",
						},
					},
				},
			},
			wantErr: errors.New("unknown transform type: unknown"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildTransforms(context.Background(), tt.args.backend)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildRuleConditions(t *testing.T) {
	pathTypeExact := networking.PathTypeExact
	pathTypePrefix := networking.PathTypePrefix
	pathTypeImplementationSpecific := networking.PathTypeImplementationSpecific

	tests := []struct {
		name     string
		ing      ClassifiedIngress
		rule     networking.IngressRule
		path     networking.HTTPIngressPath
		backend  EnhancedBackend
		want     []elbv2model.RuleCondition
		wantErr  bool
		errorMsg string
	}{
		{
			name: "host only",
			ing:  MockIngress,
			rule: networking.IngressRule{
				Host: "example.com",
			},
			path: networking.HTTPIngressPath{},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{"example.com"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "host only, with wildcard",
			ing:  MockIngress,
			rule: networking.IngressRule{
				Host: "*.example.com",
			},
			path: networking.HTTPIngressPath{},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{"*.example.com"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "path only (exact path type)",
			ing:  MockIngress,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{
				Path:     "/exact",
				PathType: &pathTypeExact,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/exact"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "path only (prefix path type)",
			ing:  MockIngress,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{
				Path:     "/prefix",
				PathType: &pathTypePrefix,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/prefix", "/prefix/*"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "path only (ImplementationSpecific path type)",
			ing:  MockIngress,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{
				Path:     "/impl",
				PathType: &pathTypeImplementationSpecific,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/impl"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "path only (ImplementationSpecific path type, with regex)",
			ing:  MockIngressWithUseRegexPathMatch,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{
				Path:     "/^api/.+$",
				PathType: &pathTypeImplementationSpecific,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						RegexValues: []string{"^api/.+$"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "both host and path",
			ing:  MockIngress,
			rule: networking.IngressRule{
				Host: "example.com",
			},
			path: networking.HTTPIngressPath{
				Path:     "/path",
				PathType: &pathTypeExact,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{"example.com"},
					},
				},
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/path"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "host and host header condition (values)",
			ing:  MockIngress,
			rule: networking.IngressRule{
				Host: "example.com",
			},
			path: networking.HTTPIngressPath{},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldHostHeader,
						HostHeaderConfig: &HostHeaderConditionConfig{
							Values: []string{"another-example.com"},
						},
					},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						Values: []string{"example.com", "another-example.com"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "host header condition only (regex values)",
			ing:  MockIngress,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldHostHeader,
						HostHeaderConfig: &HostHeaderConditionConfig{
							RegexValues: []string{"^.+\\.example\\.com$"},
						},
					},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldHostHeader,
					HostHeaderConfig: &elbv2model.HostHeaderConditionConfig{
						RegexValues: []string{"^.+\\.example\\.com$"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "host and host header condition (mixed values and regex values)",
			ing:  MockIngress,
			rule: networking.IngressRule{
				Host: "example.com",
			},
			path: networking.HTTPIngressPath{},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldHostHeader,
						HostHeaderConfig: &HostHeaderConditionConfig{
							RegexValues: []string{"^another-example.com$", "^yet-another-example.com$"},
						},
					},
				},
			},
			want:     nil,
			wantErr:  true,
			errorMsg: "host condition must specify exactly one of Values and RegexValues, got both Values [example.com] and RegexValues [^another-example.com$ ^yet-another-example.com$]",
		},
		{
			name: "path and path condition (values)",
			ing:  MockIngress,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{
				Path:     "/api",
				PathType: &pathTypePrefix,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldPathPattern,
						PathPatternConfig: &PathPatternConditionConfig{
							Values: []string{"/another-api/*"},
						},
					},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						Values: []string{"/api", "/api/*", "/another-api/*"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "path and path condition (regex values)",
			ing:  MockIngressWithUseRegexPathMatch,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{
				Path:     "/^api/.+$",
				PathType: &pathTypeImplementationSpecific,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldPathPattern,
						PathPatternConfig: &PathPatternConditionConfig{
							RegexValues: []string{"^/another\\-api/.+$"},
						},
					},
				},
			},
			want: []elbv2model.RuleCondition{
				{
					Field: elbv2model.RuleConditionFieldPathPattern,
					PathPatternConfig: &elbv2model.PathPatternConditionConfig{
						RegexValues: []string{"^api/.+$", "^/another\\-api/.+$"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "path and path condition (mixed values and regex values)",
			ing:  MockIngressWithUseRegexPathMatch,
			rule: networking.IngressRule{},
			path: networking.HTTPIngressPath{
				Path:     "/^api/.+$",
				PathType: &pathTypeImplementationSpecific,
			},
			backend: EnhancedBackend{
				Conditions: []RuleCondition{
					{
						Field: RuleConditionFieldPathPattern,
						PathPatternConfig: &PathPatternConditionConfig{
							Values: []string{"/another-api/*", "/yet-another-api/*"},
						},
					},
				},
			},
			want:     nil,
			wantErr:  true,
			errorMsg: "path condition must specify exactly one of Values and RegexValues, got both Values [/another-api/* /yet-another-api/*] and RegexValues [^api/.+$]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
			}
			got, err := task.buildRuleConditions(context.Background(), tt.ing, tt.rule, tt.path, tt.backend)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.EqualError(t, err, tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
