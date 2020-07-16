package build

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"strconv"
	"strings"
)

const (
	DeregistrationDelayTimeoutSecondsKey = "deregistration_delay.timeout_seconds"
	SlowStartDurationSecondsKey          = "slow_start.duration_seconds"
	StickinessEnabledKey                 = "stickiness.enabled"
	StickinessTypeKey                    = "stickiness.type"
	StickinessLbCookieDurationSecondsKey = "stickiness.lb_cookie.duration_seconds"
	ProxyProtocolV2Enabled               = "proxy_protocol_v2.enabled"

	DefaultDeregistrationDelayTimeoutSeconds = 300
	DefaultSlowStartDurationSeconds          = 0
	DefaultStickinessEnabled                 = false
	DefaultStickinessType                    = api.TargetGroupStickinessTypeLBCookie
	DefaultStickinessLbCookieDurationSeconds = 86400
)

func (b *defaultBuilder) buildTGAttributes(ctx context.Context, ingAnnotations map[string]string, svcAnnotations map[string]string) (api.TargetGroupAttributes, error) {
	var attributes []string
	b.annotationParser.ParseStringSliceAnnotation(k8s.AnnotationSuffixTargetGroupAttributes,
		&attributes, svcAnnotations, ingAnnotations)

	var invalidAttributes []string
	var elbv2Attrs []*elbv2.TargetGroupAttribute
	for _, attribute := range attributes {
		parts := strings.Split(attribute, "=")
		if len(parts) != 2 {
			invalidAttributes = append(invalidAttributes, attribute)
		}
		elbv2Attrs = append(elbv2Attrs, &elbv2.TargetGroupAttribute{
			Key:   aws.String(strings.TrimSpace(parts[0])),
			Value: aws.String(strings.TrimSpace(parts[1])),
		})
	}

	if len(invalidAttributes) > 0 {
		return api.TargetGroupAttributes{},
			errors.Errorf("unable to parse `%s` into Key=Value pair", strings.Join(invalidAttributes, ", "))
	}
	tgAttributes, unknown, err := ParseTargetGroupAttributes(elbv2Attrs)
	if err != nil {
		return api.TargetGroupAttributes{}, err
	}
	if len(unknown) != 0 {
		return api.TargetGroupAttributes{}, errors.Errorf("unknown targetGroup attributes: %v", unknown)
	}
	return tgAttributes, nil
}

func ParseTargetGroupAttributes(attributes []*elbv2.TargetGroupAttribute) (api.TargetGroupAttributes, []string, error) {
	tgAttributes := api.TargetGroupAttributes{
		DeregistrationDelay: api.TargetGroupDeregistrationDelayAttributes{
			TimeoutSeconds: DefaultDeregistrationDelayTimeoutSeconds,
		},
		SlowStart: api.TargetGroupSlowStartAttributes{
			DurationSeconds: DefaultSlowStartDurationSeconds,
		},
		Stickiness: api.TargetGroupStickinessAttributes{
			Enabled: DefaultStickinessEnabled,
			Type:    DefaultStickinessType,
			LBCookie: api.LBCookieConfig{
				DurationSeconds: DefaultStickinessLbCookieDurationSeconds,
			},
		},
	}

	var unknownAttrs []string
	var err error
	for _, attr := range attributes {
		attrValue := aws.StringValue(attr.Value)
		switch attrKey := aws.StringValue(attr.Key); attrKey {
		case DeregistrationDelayTimeoutSecondsKey:
			tgAttributes.DeregistrationDelay.TimeoutSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return tgAttributes, unknownAttrs, errors.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
			if tgAttributes.DeregistrationDelay.TimeoutSeconds < 0 || tgAttributes.DeregistrationDelay.TimeoutSeconds > 3600 {
				return tgAttributes, unknownAttrs, errors.Errorf("%s must be within 0-3600 seconds, not %v", attrKey, attrValue)
			}
		case SlowStartDurationSecondsKey:
			tgAttributes.SlowStart.DurationSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return tgAttributes, unknownAttrs, errors.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
			if (tgAttributes.SlowStart.DurationSeconds < 30 || tgAttributes.SlowStart.DurationSeconds > 900) && tgAttributes.SlowStart.DurationSeconds != 0 {
				return tgAttributes, unknownAttrs, errors.Errorf("%s must be within 30-900 seconds, not %v", attrKey, attrValue)
			}
		case StickinessEnabledKey:
			tgAttributes.Stickiness.Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return tgAttributes, unknownAttrs, errors.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
		case StickinessTypeKey:
			tgAttributes.Stickiness.Type, err = api.ParseTargetGroupStickinessType(attrValue)
			if err != nil {
				return tgAttributes, unknownAttrs, err
			}
		case StickinessLbCookieDurationSecondsKey:
			tgAttributes.Stickiness.LBCookie.DurationSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return tgAttributes, unknownAttrs, errors.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
			if tgAttributes.Stickiness.LBCookie.DurationSeconds < 1 || tgAttributes.Stickiness.LBCookie.DurationSeconds > 604800 {
				return tgAttributes, unknownAttrs, errors.Errorf("%s must be within 1-604800 seconds, not %v", attrKey, attrValue)
			}
		case ProxyProtocolV2Enabled:
			tgAttributes.ProxyProtocolV2.Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return tgAttributes, unknownAttrs, errors.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
		default:
			unknownAttrs = append(unknownAttrs, attrKey)
		}
	}
	return tgAttributes, unknownAttrs, nil
}
