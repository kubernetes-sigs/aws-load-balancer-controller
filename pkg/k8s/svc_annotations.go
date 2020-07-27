package k8s

const (
	// prefixes service.beta.kubernetes.io, service.kubernetes.io
	// Service Annotation suffixex
	ServiceAnnotationLoadBalancerType                          = "aws-load-balancer-type"
	ServiceAnnotationLoadBalancerInternal                      = "aws-load-balancer-internal"
	ServiceAnnotationLoadBalancerProxyProtocol                 = "aws-load-balancer-proxy-protocol"
	ServiceAnnotationLoadBalancerAccessLogEmitInterval         = "aws-load-balancer-access-log-emit-interval"
	ServiceAnnotationLoadBalancerAccessLogEnabled              = "aws-load-balancer-access-log-enabled"
	ServiceAnnotationLoadBalancerAccessLogS3BucketName         = "aws-load-balancer-access-log-s3-bucket-name"
	ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix       = "aws-load-balancer-access-log-s3-bucket-prefix"
	ServiceAnnotationLoadBalancerConnectionDrainingEnabled     = "aws-load-balancer-connection-draining-enabled"
	ServiceAnnotationLoadBalancerConnectionDrainingTimeout     = "aws-load-balancer-connection-draining-timeout"
	ServiceAnnotationLoadBalancerConnectionIdleTimeout         = "aws-load-balancer-connection-idle-timeout"
	ServiceAnnotationLoadBalancerCrossZoneLoadBalancingEnabled = "aws-load-balancer-cross-zone-load-balancing-enabled"
	ServiceAnnotationLoadBalancerExtraSecurityGroups           = "aws-load-balancer-extra-security-groups"
	ServiceAnnotationLoadBalancerSecurityGroups                = "aws-load-balancer-security-groups"
	ServiceAnnotationLoadBalancerCertificate                   = "aws-load-balancer-ssl-cert"
	ServiceAnnotationLoadBalancerSSLPorts                      = "aws-load-balancer-ssl-ports"
	ServiceAnnotationLoadBalancerSSLNegotiationPolicy          = "aws-load-balancer-ssl-negotiation-policy"
	ServiceAnnotationLoadBalancerBEProtocol                    = "aws-load-balancer-backend-protocol"
	ServiceAnnotationLoadBalancerAdditionalTags                = "aws-load-balancer-additional-resource-tags"
	ServiceAnnotationLoadBalancerHCHealthyThreshold            = "aws-load-balancer-healthcheck-healthy-threshold"
	ServiceAnnotationLoadBalancerHCUnhealthyThreshold          = "aws-load-balancer-healthcheck-unhealthy-threshold"
	ServiceAnnotationLoadBalancerHCTimeout                     = "aws-load-balancer-healthcheck-timeout"
	ServiceAnnotationLoadBalancerHCInterval                    = "aws-load-balancer-healthcheck-interval"
	ServiceAnnotationLoadBalancerHealthCheckProtocol           = "aws-load-balancer-healthcheck-protocol"
	ServiceAnnotationLoadBalancerHealthCheckPort               = "aws-load-balancer-healthcheck-port"
	ServiceAnnotationLoadBalancerHealthCheckPath               = "aws-load-balancer-healthcheck-path"
	ServiceAnnotationLoadBalancerEIPAllocations                = "aws-load-balancer-eip-allocations"
	ServiceAnnotationLoadBalancerTargetNodeLabels              = "aws-load-balancer-target-node-labels"
)

func NewServiceAnnotationParser(prefixes ...string) AnnotationParser {
	parser := &serviceAnnotationParser{}
	for _, pfx := range prefixes {
		suffixParser := NewSuffixAnnotationParser(pfx)
		parser.suffixParsers = append(parser.suffixParsers, suffixParser)
	}
	return parser
}

var _ AnnotationParser = &serviceAnnotationParser{}

type serviceAnnotationParser struct {
	suffixParsers []AnnotationParser
}

func (p *serviceAnnotationParser) ParseStringAnnotation(suffix string, value *string, annotations ...map[string]string) bool {
	for _, parser := range p.suffixParsers {
		if parser.ParseStringAnnotation(suffix, value, annotations...) {
			return true
		}
	}
	return false
}

func (p *serviceAnnotationParser) ParseInt64Annotation(suffix string, value *int64, annotations ...map[string]string) (bool, error) {
	for _, parser := range p.suffixParsers {
		if exists, err := parser.ParseInt64Annotation(suffix, value, annotations...); exists {
			return exists, err
		}
	}
	return false, nil
}

func (p *serviceAnnotationParser) ParseStringSliceAnnotation(suffix string, value *[]string, annotations ...map[string]string) bool {
	for _, parser := range p.suffixParsers {
		if parser.ParseStringSliceAnnotation(suffix, value, annotations...) {
			return true
		}
	}
	return false
}

func (p *serviceAnnotationParser) ParseJSONAnnotation(suffix string, value interface{}, annotations ...map[string]string) (bool, error) {
	for _, parser := range p.suffixParsers {
		if exists, err := parser.ParseJSONAnnotation(suffix, value, annotations...); exists {
			return exists, err
		}
	}
	return false, nil
}

func (p *serviceAnnotationParser) ParseStringMapAnnotation(suffix string, value *map[string]string, annotations ...map[string]string) bool {
	for _, parser := range p.suffixParsers {
		if parser.ParseStringMapAnnotation(suffix, value, annotations...) {
			return true
		}
	}
	return false
}
