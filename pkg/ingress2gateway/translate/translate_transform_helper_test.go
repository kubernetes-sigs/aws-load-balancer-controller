package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestTranslateTransforms(t *testing.T) {
	tests := []struct {
		name       string
		transforms []ingress.Transform
		wantNil    bool
		check      func(t *testing.T, filter *gwv1.HTTPRouteFilter)
	}{
		{
			name:       "nil transforms returns nil",
			transforms: nil,
			wantNil:    true,
		},
		{
			name:       "empty transforms returns nil",
			transforms: []ingress.Transform{},
			wantNil:    true,
		},
		{
			name: "url-rewrite with static replace produces ReplaceFullPath",
			transforms: []ingress.Transform{{
				Type: ingress.TransformTypeUrlRewrite,
				UrlRewriteConfig: &ingress.RewriteConfigObject{
					Rewrites: []ingress.RewriteConfig{{Regex: ".*", Replace: "/new/path"}},
				},
			}},
			check: func(t *testing.T, filter *gwv1.HTTPRouteFilter) {
				assert.Equal(t, gwv1.HTTPRouteFilterURLRewrite, filter.Type)
				require.NotNil(t, filter.URLRewrite)
				require.NotNil(t, filter.URLRewrite.Path)
				assert.Equal(t, gwv1.FullPathHTTPPathModifier, filter.URLRewrite.Path.Type)
				assert.Equal(t, "/new/path", *filter.URLRewrite.Path.ReplaceFullPath)
				assert.Nil(t, filter.URLRewrite.Hostname)
			},
		},
		{
			name: "url-rewrite with capture group reference is skipped",
			transforms: []ingress.Transform{{
				Type: ingress.TransformTypeUrlRewrite,
				UrlRewriteConfig: &ingress.RewriteConfigObject{
					Rewrites: []ingress.RewriteConfig{{Regex: "^\\/api\\/(.+)$", Replace: "/$1"}},
				},
			}},
			wantNil: true,
		},
		{
			name: "host-header-rewrite with static replacement produces Hostname",
			transforms: []ingress.Transform{{
				Type: ingress.TransformTypeHostHeaderRewrite,
				HostHeaderRewriteConfig: &ingress.RewriteConfigObject{
					Rewrites: []ingress.RewriteConfig{{Regex: ".*", Replace: "example.org"}},
				},
			}},
			check: func(t *testing.T, filter *gwv1.HTTPRouteFilter) {
				assert.Equal(t, gwv1.HTTPRouteFilterURLRewrite, filter.Type)
				require.NotNil(t, filter.URLRewrite)
				require.NotNil(t, filter.URLRewrite.Hostname)
				assert.Equal(t, gwv1.PreciseHostname("example.org"), *filter.URLRewrite.Hostname)
				assert.Nil(t, filter.URLRewrite.Path)
			},
		},
		{
			name: "host-header-rewrite with capture group reference is skipped",
			transforms: []ingress.Transform{{
				Type: ingress.TransformTypeHostHeaderRewrite,
				HostHeaderRewriteConfig: &ingress.RewriteConfigObject{
					Rewrites: []ingress.RewriteConfig{{Regex: "^(.+)\\.example\\.com$", Replace: "$1.internal"}},
				},
			}},
			wantNil: true,
		},
		{
			name: "both url-rewrite and host-header-rewrite with static replacements combined",
			transforms: []ingress.Transform{
				{
					Type: ingress.TransformTypeUrlRewrite,
					UrlRewriteConfig: &ingress.RewriteConfigObject{
						Rewrites: []ingress.RewriteConfig{{Regex: ".*", Replace: "/new"}},
					},
				},
				{
					Type: ingress.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &ingress.RewriteConfigObject{
						Rewrites: []ingress.RewriteConfig{{Regex: ".*", Replace: "foo.com"}},
					},
				},
			},
			check: func(t *testing.T, filter *gwv1.HTTPRouteFilter) {
				require.NotNil(t, filter.URLRewrite.Path)
				assert.Equal(t, gwv1.FullPathHTTPPathModifier, filter.URLRewrite.Path.Type)
				assert.Equal(t, "/new", *filter.URLRewrite.Path.ReplaceFullPath)
				require.NotNil(t, filter.URLRewrite.Hostname)
				assert.Equal(t, gwv1.PreciseHostname("foo.com"), *filter.URLRewrite.Hostname)
			},
		},
		{
			name: "url-rewrite with capture group skipped but host-header-rewrite static kept",
			transforms: []ingress.Transform{
				{
					Type: ingress.TransformTypeUrlRewrite,
					UrlRewriteConfig: &ingress.RewriteConfigObject{
						Rewrites: []ingress.RewriteConfig{{Regex: "^\\/api\\/(.+)$", Replace: "/$1"}},
					},
				},
				{
					Type: ingress.TransformTypeHostHeaderRewrite,
					HostHeaderRewriteConfig: &ingress.RewriteConfigObject{
						Rewrites: []ingress.RewriteConfig{{Regex: ".*", Replace: "backend.internal"}},
					},
				},
			},
			check: func(t *testing.T, filter *gwv1.HTTPRouteFilter) {
				assert.Nil(t, filter.URLRewrite.Path)
				require.NotNil(t, filter.URLRewrite.Hostname)
				assert.Equal(t, gwv1.PreciseHostname("backend.internal"), *filter.URLRewrite.Hostname)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := translateTransforms(tt.transforms)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			tt.check(t, result)
		})
	}
}

func TestHasCaptureGroupRef(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/$1", true},
		{"$1.example.org", true},
		{"/new/$2/path", true},
		{"/static/path", false},
		{"backend.internal", false},
		{"", false},
		{"$notadigit", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, hasCaptureGroupRef(tt.input))
		})
	}
}
