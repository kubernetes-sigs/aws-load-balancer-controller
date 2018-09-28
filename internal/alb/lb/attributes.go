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
	DeletionProtectionEnabledKey = "deletion_protection.enabled"
	AccessLogsS3EnabledKey       = "access_logs.s3.enabled"
	AccessLogsS3BucketKey        = "access_logs.s3.bucket"
	AccessLogsS3PrefixKey        = "access_logs.s3.prefix"
	IdleTimeoutTimeoutSecondsKey = "idle_timeout.timeout_seconds"
	RoutingHTTP2EnabledKey       = "routing.http2.enabled"

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
	var e error
	for _, attr := range attrs {
		attrValue := aws.StringValue(attr.Value)
		switch attrKey := aws.StringValue(attr.Key); attrKey {
		case DeletionProtectionEnabledKey:
			a.DeletionProtectionEnabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
		case AccessLogsS3EnabledKey:
			a.AccessLogsS3Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
		case AccessLogsS3BucketKey:
			a.AccessLogsS3Bucket = attrValue
		case AccessLogsS3PrefixKey:
			a.AccessLogsS3Prefix = attrValue
		case IdleTimeoutTimeoutSecondsKey:
			a.IdleTimeoutTimeoutSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
			if a.IdleTimeoutTimeoutSeconds < 1 || a.IdleTimeoutTimeoutSeconds > 4000 {
				return a, fmt.Errorf("%s must be within 1-4000 seconds", attrKey)
			}
		case RoutingHTTP2EnabledKey:
			a.RoutingHTTP2Enabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return a, fmt.Errorf("invalid load balancer attribute value %s=%s", attrKey, attrValue)
			}
		default:
			e = NewInvalidAttribute(attrKey)
		}
	}
	return a, e
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

	current, err := NewAttributes(raw.Attributes)
	if err != nil && !IsInvalidAttribute(err) {
		return fmt.Errorf("failed parsing attributes: %v", err)
	}

	changeSet := attributesChangeSet(current, desired)
	if len(changeSet) > 0 {
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
func attributesChangeSet(a, b *Attributes) (changeSet []*elbv2.LoadBalancerAttribute) {
	if a.DeletionProtectionEnabled != b.DeletionProtectionEnabled {
		changeSet = append(changeSet, lbAttribute(DeletionProtectionEnabledKey, fmt.Sprintf("%v", b.DeletionProtectionEnabled)))
	}

	if a.AccessLogsS3Enabled != b.AccessLogsS3Enabled {
		changeSet = append(changeSet, lbAttribute(AccessLogsS3EnabledKey, fmt.Sprintf("%v", b.AccessLogsS3Enabled)))
	}

	if a.AccessLogsS3Bucket != b.AccessLogsS3Bucket {
		changeSet = append(changeSet, lbAttribute(AccessLogsS3BucketKey, b.AccessLogsS3Bucket))
	}

	if a.AccessLogsS3Prefix != b.AccessLogsS3Prefix {
		changeSet = append(changeSet, lbAttribute(AccessLogsS3PrefixKey, b.AccessLogsS3Prefix))
	}

	if a.IdleTimeoutTimeoutSeconds != b.IdleTimeoutTimeoutSeconds {
		changeSet = append(changeSet, lbAttribute(IdleTimeoutTimeoutSecondsKey, fmt.Sprintf("%v", b.IdleTimeoutTimeoutSeconds)))
	}

	if a.RoutingHTTP2Enabled != b.RoutingHTTP2Enabled {
		changeSet = append(changeSet, lbAttribute(RoutingHTTP2EnabledKey, fmt.Sprintf("%v", b.RoutingHTTP2Enabled)))
	}

	return
}

func lbAttribute(k, v string) *elbv2.LoadBalancerAttribute {
	return &elbv2.LoadBalancerAttribute{Key: aws.String(k), Value: aws.String(v)}
}

// NewInvalidAttribute returns a new InvalidAttribute  error
func NewInvalidAttribute(name string) error {
	return InvalidAttribute{
		Name: fmt.Sprintf("the load balancer attribute %v is not valid", name),
	}
}

// InvalidAttribute error
type InvalidAttribute struct {
	Name string
}

func (e InvalidAttribute) Error() string {
	return e.Name
}

// IsInvalidAttribute checks if the err is from an invalid attribute
func IsInvalidAttribute(e error) bool {
	_, ok := e.(InvalidAttribute)
	return ok
}
