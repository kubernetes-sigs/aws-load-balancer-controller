package logdelivery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

// Log types that load balancers deliver through CloudWatch Logs vended log delivery.
const (
	LogTypeALBAccessLogs      = "ALB_ACCESS_LOGS"
	LogTypeALBConnectionLogs  = "ALB_CONNECTION_LOGS"
	LogTypeALBHealthCheckLogs = "ALB_HEALTH_CHECK_LOGS"
	LogTypeNLBAccessLogs      = "NLB_ACCESS_LOGS"
)

// Delivery destination types, as CloudWatch Logs names them.
const (
	DestinationTypeCloudWatchLogs = "CWL"
	DestinationTypeS3             = "S3"
	DestinationTypeFirehose       = "FH"
)

var logTypesByLoadBalancerType = map[elbv2model.LoadBalancerType][]string{
	elbv2model.LoadBalancerTypeApplication: {LogTypeALBAccessLogs, LogTypeALBConnectionLogs, LogTypeALBHealthCheckLogs},
	elbv2model.LoadBalancerTypeNetwork:     {LogTypeNLBAccessLogs},
}

// output formats that each destination type accepts for load balancer logs.
var outputFormatsByDestinationType = map[string][]string{
	DestinationTypeCloudWatchLogs: {"plain", "json"},
	DestinationTypeFirehose:       {"plain", "json", "raw"},
	DestinationTypeS3:             {"plain", "json", "w3c", "parquet"},
}

var allowedFieldDelimiters = []string{"\t", " ", ","}

// Config is one log delivery requested for a load balancer, as users write it in annotations and CRDs.
type Config struct {
	// LogType is the type of logs to deliver.
	LogType string `json:"logType"`

	// DestinationARN is the ARN of a log group, S3 bucket (optionally followed by a prefix) or Firehose delivery stream.
	DestinationARN *string `json:"destinationArn,omitempty"`

	// DeliveryDestinationARN is the ARN of an existing delivery destination.
	DeliveryDestinationARN *string `json:"deliveryDestinationArn,omitempty"`

	// OutputFormat is the format of the delivered logs. Only valid with DestinationARN.
	OutputFormat *string `json:"outputFormat,omitempty"`

	// FieldDelimiter separates fields in plain and w3c output.
	FieldDelimiter *string `json:"fieldDelimiter,omitempty"`

	// S3DeliveryConfiguration configures the S3 object path.
	S3DeliveryConfiguration *S3DeliveryConfiguration `json:"s3DeliveryConfiguration,omitempty"`
}

// BuildLogDeliveries validates configs for a load balancer and adds one LogDelivery resource per config to the stack.
func BuildLogDeliveries(stack core.Stack, lbType elbv2model.LoadBalancerType, lbARN core.StringToken, configs []Config, tags map[string]string) ([]*LogDelivery, error) {
	ids := sets.New[string]()
	for i, cfg := range configs {
		if err := cfg.validate(lbType); err != nil {
			return nil, errors.Wrapf(err, "invalid log delivery configuration at index %d", i)
		}
		id := cfg.resourceID()
		if ids.Has(id) {
			return nil, errors.Errorf("duplicate log delivery configuration for logType %s and destination %s", cfg.LogType, cfg.destination())
		}
		ids.Insert(id)
	}

	deliveries := make([]*LogDelivery, 0, len(configs))
	for _, cfg := range configs {
		deliveries = append(deliveries, NewLogDelivery(stack, cfg.resourceID(), LogDeliverySpec{
			ResourceARN:             lbARN,
			LogType:                 cfg.LogType,
			DestinationARN:          cfg.DestinationARN,
			DeliveryDestinationARN:  cfg.DeliveryDestinationARN,
			OutputFormat:            cfg.OutputFormat,
			FieldDelimiter:          cfg.FieldDelimiter,
			S3DeliveryConfiguration: cfg.S3DeliveryConfiguration,
			Tags:                    maps.Clone(tags),
		}))
	}
	return deliveries, nil
}

// DestinationType returns the delivery destination type for the ARN of a log group, S3 bucket or Firehose delivery stream.
func DestinationType(destinationARN string) (string, error) {
	parsed, err := arn.Parse(destinationARN)
	if err != nil {
		return "", errors.Errorf("invalid destinationArn %q: %v", destinationARN, err)
	}
	switch parsed.Service {
	case "logs":
		if strings.HasPrefix(parsed.Resource, "log-group:") {
			return DestinationTypeCloudWatchLogs, nil
		}
		if strings.HasPrefix(parsed.Resource, "delivery-destination:") {
			return "", errors.Errorf("destinationArn %q is a delivery destination, set it as deliveryDestinationArn instead", destinationARN)
		}
	case "s3":
		bucket, _, _ := strings.Cut(parsed.Resource, "/")
		if parsed.Region == "" && parsed.AccountID == "" && bucket != "" && !strings.ContainsAny(bucket, ":*") {
			return DestinationTypeS3, nil
		}
	case "firehose":
		if strings.HasPrefix(parsed.Resource, "deliverystream/") {
			return DestinationTypeFirehose, nil
		}
	}
	return "", errors.Errorf("destinationArn %q must be a CloudWatch Logs log group, an S3 bucket or a Firehose delivery stream", destinationARN)
}

func (cfg Config) validate(lbType elbv2model.LoadBalancerType) error {
	if !slices.Contains(logTypesByLoadBalancerType[lbType], cfg.LogType) {
		return errors.Errorf("logType %q is not supported for %s load balancers, supported values: %v", cfg.LogType, lbType, logTypesByLoadBalancerType[lbType])
	}
	hasDestination := cfg.DestinationARN != nil && *cfg.DestinationARN != ""
	hasDeliveryDestination := cfg.DeliveryDestinationARN != nil && *cfg.DeliveryDestinationARN != ""
	if hasDestination == hasDeliveryDestination {
		return errors.New("exactly one of destinationArn or deliveryDestinationArn must be set")
	}
	if cfg.FieldDelimiter != nil && !slices.Contains(allowedFieldDelimiters, *cfg.FieldDelimiter) {
		return errors.Errorf("fieldDelimiter %q is not supported, supported values: %q", *cfg.FieldDelimiter, allowedFieldDelimiters)
	}

	if hasDeliveryDestination {
		parsed, err := arn.Parse(*cfg.DeliveryDestinationARN)
		if err != nil || parsed.Service != "logs" || !strings.HasPrefix(parsed.Resource, "delivery-destination:") {
			return errors.Errorf("deliveryDestinationArn %q must be the ARN of a CloudWatch Logs delivery destination", *cfg.DeliveryDestinationARN)
		}
		if cfg.OutputFormat != nil {
			return errors.New("outputFormat is set by the delivery destination and cannot be used with deliveryDestinationArn")
		}
		return nil
	}

	destinationType, err := DestinationType(*cfg.DestinationARN)
	if err != nil {
		return err
	}
	if cfg.OutputFormat != nil && !slices.Contains(outputFormatsByDestinationType[destinationType], *cfg.OutputFormat) {
		return errors.Errorf("outputFormat %q is not supported for %s destinations, supported values: %v", *cfg.OutputFormat, destinationType, outputFormatsByDestinationType[destinationType])
	}
	if cfg.FieldDelimiter != nil && cfg.OutputFormat != nil && *cfg.OutputFormat != "plain" && *cfg.OutputFormat != "w3c" {
		return errors.Errorf("fieldDelimiter can only be used with plain or w3c outputFormat, got %q", *cfg.OutputFormat)
	}
	if cfg.S3DeliveryConfiguration != nil && destinationType != DestinationTypeS3 {
		return errors.New("s3DeliveryConfiguration can only be used with S3 destinations")
	}
	return nil
}

// destination identifies where the logs go, regardless of how the destination is referenced.
func (cfg Config) destination() string {
	if cfg.DeliveryDestinationARN != nil && *cfg.DeliveryDestinationARN != "" {
		return *cfg.DeliveryDestinationARN
	}
	return *cfg.DestinationARN
}

// resourceID is stable for a log type and destination, so reordering configs doesn't recreate deliveries.
func (cfg Config) resourceID() string {
	hash := sha256.Sum256([]byte(cfg.destination()))
	return fmt.Sprintf("%s-%.16s", cfg.LogType, hex.EncodeToString(hash[:]))
}
