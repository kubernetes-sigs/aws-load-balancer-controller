package logdelivery

import "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"

// LogDelivery represents a CloudWatch Logs vended log delivery from a load balancer.
// The controller manages the delivery source, the delivery destination (when DestinationARN is set) and the delivery.
type LogDelivery struct {
	core.ResourceMeta `json:"-"`

	// desired state of LogDelivery
	Spec LogDeliverySpec `json:"spec"`
}

// NewLogDelivery constructs new LogDelivery resource.
func NewLogDelivery(stack core.Stack, id string, spec LogDeliverySpec) *LogDelivery {
	d := &LogDelivery{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::Logs::Delivery", id),
		Spec:         spec,
	}
	stack.AddResource(d)
	d.registerDependencies(stack)
	return d
}

// register dependencies for LogDelivery.
func (d *LogDelivery) registerDependencies(stack core.Stack) {
	for _, dep := range d.Spec.ResourceARN.Dependencies() {
		stack.AddDependency(dep, d)
	}
}

// LogDeliverySpec defines the desired state of LogDelivery.
type LogDeliverySpec struct {
	// ResourceARN is the ARN of the load balancer that sends the logs.
	ResourceARN core.StringToken `json:"resourceARN"`

	// LogType is the type of logs to deliver, such as ALB_ACCESS_LOGS.
	LogType string `json:"logType"`

	// DestinationARN is the ARN of a log group, S3 bucket or Firehose delivery stream.
	// The controller creates and owns a delivery destination for it.
	// +optional
	DestinationARN *string `json:"destinationARN,omitempty"`

	// DeliveryDestinationARN is the ARN of an existing delivery destination that the controller does not own.
	// +optional
	DeliveryDestinationARN *string `json:"deliveryDestinationARN,omitempty"`

	// OutputFormat is the format of the delivered logs. It applies to controller-owned delivery destinations only.
	// +optional
	OutputFormat *string `json:"outputFormat,omitempty"`

	// FieldDelimiter separates fields in plain and w3c output.
	// +optional
	FieldDelimiter *string `json:"fieldDelimiter,omitempty"`

	// S3DeliveryConfiguration configures the S3 object path.
	// +optional
	S3DeliveryConfiguration *S3DeliveryConfiguration `json:"s3DeliveryConfiguration,omitempty"`

	// Tags are applied to the AWS resources the controller creates for this delivery.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// S3DeliveryConfiguration configures the S3 object path of a delivery.
type S3DeliveryConfiguration struct {
	// SuffixPath is appended to the service-defined path, such as "{yyyy}/{MM}/{dd}".
	// +optional
	SuffixPath *string `json:"suffixPath,omitempty"`

	// EnableHiveCompatiblePath renders path variables as key=value.
	// +optional
	EnableHiveCompatiblePath *bool `json:"enableHiveCompatiblePath,omitempty"`
}
