package lb

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	api "k8s.io/api/core/v1"
)

const (
	DeletionProtectionEnabledString = "deletion_protection.enabled"
	AccessLogsS3EnabledString       = "access_logs.s3.enabled"
	AccessLogsS3BucketString        = "access_logs.s3.bucket"
	AccessLogsS3PrefixString        = "access_logs.s3.prefix"
	IdleTimeoutTimeoutSecondsString = "idle_timeout.timeout_seconds"
	RoutingHTTP2EnabledString       = "routing.http2.enabled"

	DeletionProtectionEnabled = false
	AccessLogsS3Enabled       = false
	AccessLogsS3Bucket        = ""
	AccessLogsS3Prefix        = ""
	IdleTimeoutTimeoutSeconds = 60
	RoutingHTTP2Enabled       = true
)

// Attributes represents the desired state of attributes for a load balancer.
type Attributes struct {
	// LbArn is the ARN of the load balancer
	LbArn string

	// DeletionProtectionEnabled: deletion_protection.enabled - Indicates whether deletion protection
	// is enabled. The value is true or false. The default is false.
	DeletionProtectionEnabled bool

	// AccessLogsS3Enabled: access_logs.s3.enabled - Indicates whether access logs are enabled.
	// The value is true or false. The default is false.
	AccessLogsS3Enabled bool

	// AccessLogsS3Bucket: access_logs.s3.bucket - The name of the S3 bucket for the access logs.
	// This attribute is required if access logs are enabled. The bucket must
	// exist in the same region as the load balancer and have a bucket policy
	// that grants Elastic Load Balancing permissions to write to the bucket.
	AccessLogsS3Bucket string

	// AccessLogsS3Prefix: access_logs.s3.prefix - The prefix for the location in the S3 bucket
	// for the access logs.
	AccessLogsS3Prefix string

	// IdleTimeoutTimeoutSeconds: idle_timeout.timeout_seconds - The idle timeout value, in seconds. The
	// valid range is 1-4000 seconds. The default is 60 seconds.
	IdleTimeoutTimeoutSeconds int64

	// RoutingHTTP2Enabled: routing.http2.enabled - Indicates whether HTTP/2 is enabled. The value
	// is true or false. The default is true.
	RoutingHTTP2Enabled bool
}

func NewAttributes(attrs []*elbv2.LoadBalancerAttribute) (a *Attributes, err error) {
	a = &Attributes{
		DeletionProtectionEnabled: DeletionProtectionEnabled,
		AccessLogsS3Enabled:       AccessLogsS3Enabled,
		AccessLogsS3Bucket:        AccessLogsS3Bucket,
		AccessLogsS3Prefix:        AccessLogsS3Prefix,
		IdleTimeoutTimeoutSeconds: IdleTimeoutTimeoutSeconds,
		RoutingHTTP2Enabled:       RoutingHTTP2Enabled,
	}
	for _, attr := range attrs {
		switch aws.StringValue(attr.Key) {
		case DeletionProtectionEnabledString:
			a.DeletionProtectionEnabled, err = strconv.ParseBool(aws.StringValue(attr.Value))
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", aws.StringValue(attr.Key), aws.StringValue(attr.Value))
			}
		case AccessLogsS3EnabledString:
			a.AccessLogsS3Enabled, err = strconv.ParseBool(aws.StringValue(attr.Value))
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", aws.StringValue(attr.Key), aws.StringValue(attr.Value))
			}
		case AccessLogsS3BucketString:
			a.AccessLogsS3Bucket = aws.StringValue(attr.Value)
		case AccessLogsS3PrefixString:
			a.AccessLogsS3Prefix = aws.StringValue(attr.Value)
		case IdleTimeoutTimeoutSecondsString:
			a.IdleTimeoutTimeoutSeconds, err = strconv.ParseInt(aws.StringValue(attr.Value), 10, 64)
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", aws.StringValue(attr.Key), aws.StringValue(attr.Value))
			}
			if a.IdleTimeoutTimeoutSeconds < 1 || a.IdleTimeoutTimeoutSeconds > 4000 {
				return a, fmt.Errorf("%s must be within 1-4000 seconds", aws.StringValue(attr.Key))
			}
		case RoutingHTTP2EnabledString:
			a.RoutingHTTP2Enabled, err = strconv.ParseBool(aws.StringValue(attr.Value))
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", aws.StringValue(attr.Key), aws.StringValue(attr.Value))
			}
		default:
			return a, fmt.Errorf("invalid load balancer attribute %s", aws.StringValue(attr.Key))
		}
	}
	return a, nil
}

// AttributesController provides functionality to manage Attributes
type AttributesController interface {
	// Reconcile ensures the load balancer attributes in AWS matches the state specified by the ingress configuration.
	Reconcile(context.Context, *Attributes) error
}

// NewAttributesController constructs a new attributes controller
func NewAttributesController(elbv2 elbv2iface.ELBV2API) AttributesController {
	return &attributesController{
		elbv2: elbv2,
	}
}

type attributesController struct {
	elbv2 elbv2iface.ELBV2API
}

func (c *attributesController) Reconcile(ctx context.Context, desired *Attributes) error {
	raw, err := c.elbv2.DescribeLoadBalancerAttributes(&elbv2.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(desired.LbArn),
	})

	if err != nil {
		return fmt.Errorf("failed to retrieve attributes from ELBV2 in AWS: %s", err.Error())
	}

	current, _ := NewAttributes(raw.Attributes)

	changeSet, ok := attributesChangeSet(current, desired)
	if ok {
		albctx.GetLogger(ctx).Infof("Modifying ELBV2 attributes to %v.", log.Prettify(changeSet))
		_, err = c.elbv2.ModifyLoadBalancerAttributes(&elbv2.ModifyLoadBalancerAttributesInput{
			LoadBalancerArn: aws.String(desired.LbArn),
			Attributes:      changeSet,
		})
		if err != nil {
			eventf, ok := albctx.GetEventf(ctx)
			if ok {
				eventf(api.EventTypeWarning, "ERROR", "%s attributes modification failed: %s", desired.LbArn, err.Error())
			}
			return fmt.Errorf("failed modifying attributes: %s", err)
		}

	}
	return nil
}

// attributesChangeSet returns a list of elbv2.LoadBalancerAttribute required to change a into b
func attributesChangeSet(a, b *Attributes) (changeSet []*elbv2.LoadBalancerAttribute, ok bool) {
	if a.DeletionProtectionEnabled != b.DeletionProtectionEnabled && b.DeletionProtectionEnabled != DeletionProtectionEnabled {
		changeSet = append(changeSet, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(DeletionProtectionEnabledString),
			Value: aws.String(fmt.Sprintf("%v", b.DeletionProtectionEnabled)),
		})
	}

	if a.AccessLogsS3Enabled != b.AccessLogsS3Enabled && b.AccessLogsS3Enabled != AccessLogsS3Enabled {
		changeSet = append(changeSet, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(AccessLogsS3EnabledString),
			Value: aws.String(fmt.Sprintf("%v", b.AccessLogsS3Enabled)),
		})
	}

	if a.AccessLogsS3Bucket != b.AccessLogsS3Bucket && b.AccessLogsS3Bucket != AccessLogsS3Bucket {
		changeSet = append(changeSet, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(AccessLogsS3BucketString),
			Value: aws.String(b.AccessLogsS3Bucket),
		})
	}

	if a.AccessLogsS3Prefix != b.AccessLogsS3Prefix && b.AccessLogsS3Prefix != AccessLogsS3Prefix {
		changeSet = append(changeSet, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(AccessLogsS3PrefixString),
			Value: aws.String(b.AccessLogsS3Prefix),
		})
	}

	if a.IdleTimeoutTimeoutSeconds != b.IdleTimeoutTimeoutSeconds && b.IdleTimeoutTimeoutSeconds != IdleTimeoutTimeoutSeconds {
		changeSet = append(changeSet, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(IdleTimeoutTimeoutSecondsString),
			Value: aws.String(fmt.Sprintf("%v", b.IdleTimeoutTimeoutSeconds)),
		})
	}

	if a.RoutingHTTP2Enabled != b.RoutingHTTP2Enabled && b.RoutingHTTP2Enabled != RoutingHTTP2Enabled {
		changeSet = append(changeSet, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(RoutingHTTP2EnabledString),
			Value: aws.String(fmt.Sprintf("%v", b.RoutingHTTP2Enabled)),
		})
	}

	if len(changeSet) > 0 {
		ok = true
	}
	return
}
