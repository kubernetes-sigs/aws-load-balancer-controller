package translate

import (
	"fmt"
	"os"
	"regexp"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// captureGroupRef matches $1, $2, etc. in replacement strings.
var captureGroupRef = regexp.MustCompile(`\$\d`)

// hasCaptureGroupRef returns true if the replacement string contains capture group references ($1, $2, etc.).
func hasCaptureGroupRef(s string) bool {
	return captureGroupRef.MatchString(s)
}

// translateTransforms converts parsed ingress Transforms into Gateway API URLRewrite filters.
// Returns a single URLRewrite filter combining path and hostname rewrites, or nil if no transforms.
//
// Only static replacements (no capture group references like $1) are supported.
// Transforms with capture group references are skipped — this is a documented gap because
// Gateway API's URLRewrite only supports ReplaceFullPath (static) and ReplacePrefixMatch,
// neither of which can represent arbitrary regex capture group back-references.
func translateTransforms(transforms []ingress.Transform) *gwv1.HTTPRouteFilter {
	if len(transforms) == 0 {
		return nil
	}

	rewrite := &gwv1.HTTPURLRewriteFilter{}
	hasRewrite := false

	for _, t := range transforms {
		switch t.Type {
		case ingress.TransformTypeUrlRewrite:
			if t.UrlRewriteConfig == nil || len(t.UrlRewriteConfig.Rewrites) == 0 {
				continue
			}
			replace := t.UrlRewriteConfig.Rewrites[0].Replace
			if hasCaptureGroupRef(replace) {
				fmt.Fprintf(os.Stderr, "WARNING: url-rewrite transform with capture group reference %q cannot be represented in Gateway API — skipping.\n", replace)
				continue
			}
			rewrite.Path = &gwv1.HTTPPathModifier{
				Type:            gwv1.FullPathHTTPPathModifier,
				ReplaceFullPath: &replace,
			}
			hasRewrite = true

		case ingress.TransformTypeHostHeaderRewrite:
			if t.HostHeaderRewriteConfig == nil || len(t.HostHeaderRewriteConfig.Rewrites) == 0 {
				continue
			}
			replace := t.HostHeaderRewriteConfig.Rewrites[0].Replace
			if hasCaptureGroupRef(replace) {
				fmt.Fprintf(os.Stderr, "WARNING: host-header-rewrite transform with capture group reference %q cannot be represented in Gateway API — skipping.\n", replace)
				continue
			}
			hostname := gwv1.PreciseHostname(replace)
			rewrite.Hostname = &hostname
			hasRewrite = true
		}
	}

	if !hasRewrite {
		return nil
	}

	return &gwv1.HTTPRouteFilter{
		Type:       gwv1.HTTPRouteFilterURLRewrite,
		URLRewrite: rewrite,
	}
}
