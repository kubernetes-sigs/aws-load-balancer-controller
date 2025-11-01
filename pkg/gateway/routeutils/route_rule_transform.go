package routeutils

import (
	"fmt"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
)

const (
	replaceWholeHostHeaderRegex           = ".*"
	replaceWholePathMinusQueryParamsRegex = "^([^?]*)"
)

func BuildRoutingRuleTransforms(gwRoute RouteDescriptor, gwRule RulePrecedence) []elbv2model.Transform {
	switch gwRoute.GetRouteKind() {
	case HTTPRouteKind:
		return buildHTTPRuleTransforms(gwRule.CommonRulePrecedence.Rule.GetRawRouteRule().(*gwv1.HTTPRouteRule), gwRule.HTTPMatch)
	default:
		return []elbv2model.Transform{}
	}
}

func buildHTTPRuleTransforms(rule *gwv1.HTTPRouteRule, httpMatch *gwv1.HTTPRouteMatch) []elbv2model.Transform {
	var transforms []elbv2model.Transform

	if rule != nil {
		for _, rf := range rule.Filters {
			if rf.URLRewrite != nil {
				if rf.URLRewrite.Path != nil {
					transforms = append(transforms, generateURLRewritePathTransform(*rf.URLRewrite.Path, httpMatch))
				}

				if rf.URLRewrite.Hostname != nil {
					transforms = append(transforms, generateHostHeaderRewriteTransform(*rf.URLRewrite.Hostname))
				}
			}
		}
	}

	return transforms
}

func generateHostHeaderRewriteTransform(hostname gwv1.PreciseHostname) elbv2model.Transform {
	return elbv2model.Transform{
		Type: elbv2model.TransformTypeHostHeaderRewrite,
		HostHeaderRewriteConfig: &elbv2model.RewriteConfigObject{
			Rewrites: []elbv2model.RewriteConfig{
				{
					Regex:   replaceWholeHostHeaderRegex,
					Replace: string(hostname),
				},
			},
		},
	}
}

func generateURLRewritePathTransform(gwPathModifier gwv1.HTTPPathModifier, httpMatch *gwv1.HTTPRouteMatch) elbv2model.Transform {
	var replacementRegex string
	var replacement string

	switch gwPathModifier.Type {
	case gwv1.FullPathHTTPPathModifier:
		// Capture just the path, not the query parameters
		replacementRegex = replaceWholePathMinusQueryParamsRegex
		replacement = *gwPathModifier.ReplaceFullPath
		break
	case gwv1.PrefixMatchHTTPPathModifier:
		replacementRegex, replacement = generatePrefixReplacementRegex(httpMatch, *gwPathModifier.ReplacePrefixMatch)
		break
	default:
		// Need to set route status as failed :blah:
		// Probably do this in the routeutils loader step and for validation.
	}
	return elbv2model.Transform{
		Type: elbv2model.TransformTypeUrlRewrite,
		UrlRewriteConfig: &elbv2model.RewriteConfigObject{
			Rewrites: []elbv2model.RewriteConfig{
				{
					Regex:   replacementRegex,
					Replace: replacement,
				},
			},
		},
	}
}

func generatePrefixReplacementRegex(httpMatch *gwv1.HTTPRouteMatch, replacement string) (string, string) {
	match := *httpMatch.Path.Value

	/*
		If we're being asked to replace a prefix with "", we still need to keep one '/' to form a valid path.
		Consider getting the path '/foo' and having the replacement string being '', we would transform '/foo' => ''
		thereby leaving an invalid path of ''. We could (in theory) do this for all replacements, e.g. replace = 'cat'
		we could transform this into '/cat' here, but tbh the user can also do this, and I'm not entirely
		sure if we could handle all possible cases.

		To explain the addition of $2, we set up an optional capture group after the initial prefix match. We only want
		to add back the value of the optional capture group when the replacement doesn't already have a '/' suffix.
		A couple examples:

		Without the capture group, e.g. (^%s)
		input path = '/foo/', prefixRegex = '(^/foo)', replacement value = '/cat/' results in '/cat//'

		To extend the example, now consider using having the capture group and always adding that to the result.
		input path = '/foo/', prefixRegex = '(^/foo(/)?)', replacement value = '/cat/$2' results in (again) '/cat//'

		Without the capture group, we would have one '/' too few.
		input path = '/foo/bar', prefixRegex = '(^/foo(/)?)', replacement value = '/cat$2' results in '/catbar'

	*/
	if replacement == "" {
		replacement = "/"
	} else if !strings.HasSuffix(replacement, "/") {
		replacement = fmt.Sprintf("%s$2", replacement)
	}

	return fmt.Sprintf("(^%s(/)?)", match), replacement
}
