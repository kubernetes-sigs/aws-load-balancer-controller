package build

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"strconv"
	"strings"
)

const (
	DeletionProtectionEnabledKey = "deletion_protection.enabled"
	AccessLogsS3EnabledKey       = "access_logs.s3.enabled"
	AccessLogsS3BucketKey        = "access_logs.s3.bucket"
	AccessLogsS3PrefixKey        = "access_logs.s3.prefix"
	IdleTimeoutTimeoutSecondsKey = "idle_timeout.timeout_seconds"
	RoutingHTTP2EnabledKey       = "routing.http2.enabled"
	CrossZoneLoadBalancing       = "load_balancing.cross_zone.enabled"

	DefaultDeletionProtectionEnabled = false
	DefaultAccessLogsS3Enabled       = false
	DefaultAccessLogsS3Bucket        = ""
	DefaultAccessLogsS3Prefix        = ""
	DefaultIdleTimeoutTimeoutSeconds = 60
	DefaultRoutingHTTP2Enabled       = true
)

func (b *defaultBuilder) buildLBAttributes(ctx context.Context, ingGroup ingress.Group) (api.LoadBalancerAttributes, error) {
	mergedAttributes := map[string]string{}
	for _, ing := range ingGroup.ActiveMembers {
		var rawAttrs []string
		_ = b.annotationParser.ParseStringSliceAnnotation(k8s.AnnotationSuffixLoadBalancerAttributes, &rawAttrs, ing.Annotations)

		var invalidAttributes []string
		for _, rawAttr := range rawAttrs {
			parts := strings.Split(rawAttr, "=")
			if len(parts) != 2 {
				invalidAttributes = append(invalidAttributes, rawAttr)
			}
			attrKey := strings.TrimSpace(parts[0])
			attrValue := strings.TrimSpace(parts[1])
			if existingAttrValue, exists := mergedAttributes[attrKey]; exists && existingAttrValue != attrValue {
				return api.LoadBalancerAttributes{}, errors.Errorf("conflicting LoadBalancer Attribute for %v: %v, %v", attrKey, existingAttrValue, attrValue)
			}
			mergedAttributes[attrKey] = attrValue
		}
		if len(invalidAttributes) > 0 {
			return api.LoadBalancerAttributes{}, fmt.Errorf("unable to parse `%s` into Key=Value pair(s)", strings.Join(invalidAttributes, ", "))
		}
	}

	elbv2Attrs := make([]*elbv2.LoadBalancerAttribute, 0, len(mergedAttributes))
	for k, v := range mergedAttributes {
		elbv2Attrs = append(elbv2Attrs, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	lbAttributes, unknown, err := ParseLoadBalancerAttributes(elbv2Attrs)
	if err != nil {
		return api.LoadBalancerAttributes{}, err
	}
	if len(unknown) != 0 {
		return api.LoadBalancerAttributes{}, errors.Errorf("unknown targetGroup attributes: %v", unknown)
	}
	return lbAttributes, nil
}

func ParseLoadBalancerAttributes(attributes []*elbv2.LoadBalancerAttribute) (api.LoadBalancerAttributes, []string, error) {
	lbAttributes := api.LoadBalancerAttributes{
		DeletionProtection: api.LoadBalancerDeletionProtectionAttributes{
			Enabled: DefaultDeletionProtectionEnabled,
		},
		AccessLogs: api.LoadBalancerAccessLogsAttributes{
			S3: api.LoadBalancerAccessLogsS3Attributes{
				Enabled: DefaultAccessLogsS3Enabled,
				Bucket:  DefaultAccessLogsS3Bucket,
				Prefix:  DefaultAccessLogsS3Prefix,
			},
		},
		IdleTimeout: api.LoadBalancerIdleTimeoutAttributes{
			TimeoutSeconds: DefaultIdleTimeoutTimeoutSeconds,
		},
		Routing: api.LoadBalancerRoutingAttributes{
			HTTP2: api.LoadBalancerRoutingHTTP2Attributes{
				Enabled: DefaultRoutingHTTP2Enabled,
			},
		},
	}

	var unknownAttrs []string
	var err error
	for _, attr := range attributes {
		attrValue := aws.StringValue(attr.Value)
		switch attrKey := aws.StringValue(attr.Key); attrKey {
		case DeletionProtectionEnabledKey:
			lbAttributes.DeletionProtection.Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return lbAttributes, unknownAttrs, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
		case AccessLogsS3EnabledKey:
			lbAttributes.AccessLogs.S3.Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return lbAttributes, unknownAttrs, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
		case AccessLogsS3BucketKey:
			lbAttributes.AccessLogs.S3.Bucket = attrValue
		case AccessLogsS3PrefixKey:
			lbAttributes.AccessLogs.S3.Prefix = attrValue
		case IdleTimeoutTimeoutSecondsKey:
			lbAttributes.IdleTimeout.TimeoutSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return lbAttributes, unknownAttrs, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
			if lbAttributes.IdleTimeout.TimeoutSeconds < 1 || lbAttributes.IdleTimeout.TimeoutSeconds > 4000 {
				return lbAttributes, unknownAttrs, fmt.Errorf("%s must be within 1-4000 seconds", attrKey)
			}
		case RoutingHTTP2EnabledKey:
			lbAttributes.Routing.HTTP2.Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return lbAttributes, unknownAttrs, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
		case CrossZoneLoadBalancing:
			lbAttributes.CrossZone.Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return lbAttributes, unknownAttrs, fmt.Errorf("invalid load valancer attribute value %s=%s, attrKey, attrValue")
			}
		default:
			unknownAttrs = append(unknownAttrs, attrKey)
		}
	}
	return lbAttributes, unknownAttrs, nil
}
