package k8s

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/algorithm"
	"strconv"
	"strings"
)

const (
	// IngressGroup
	AnnotationSuffixGroupName  = "group.name"
	AnnotationSuffixGroupOrder = "group.order"

	// LoadBalancer
	AnnotationSuffixLBSecurityGroups       = "security-groups"
	AnnotationSuffixLBInboundCIDRs         = "inbound-cidrs"
	AnnotationSuffixLoadBalancerAttributes = "load-balancer-attributes"
	AnnotationSuffixLBSchema               = "scheme"
	AnnotationSuffixLBIPAddressType        = "ip-address-type"
	AnnotationSuffixLBSubnets              = "subnets"

	// Listener
	AnnotationSuffixListenPorts    = "listen-ports"
	AnnotationSuffixCertificateARN = "certificate-arn"
	AnnotationSuffixSSLPolicy      = "ssl-policy"
	AnnotationSuffixActionPattern  = "actions.%s"

	// Authentication
	AnnotationSuffixAuthType                     string = "auth-type"
	AnnotationSuffixAuthScope                    string = "auth-scope"
	AnnotationSuffixAuthSessionCookie            string = "auth-session-cookie"
	AnnotationSuffixAuthSessionTimeout           string = "auth-session-timeout"
	AnnotationSuffixAuthOnUnauthenticatedRequest string = "auth-on-unauthenticated-request"
	AnnotationSuffixAuthIDPCognito               string = "auth-idp-cognito"
	AnnotationSuffixAuthIDPOIDC                  string = "auth-idp-oidc"

	// TargetGroup
	AnnotationSuffixTargetGroupTargetType      = "target-type"
	AnnotationSuffixTargetGroupBackendProtocol = "backend-protocol"
	AnnotationSuffixTargetGroupAttributes      = "target-group-attributes"

	// HealthCheck
	AnnotationSuffixHealthCheckProtocol                = "healthcheck-protocol"
	AnnotationSuffixHealthCheckPort                    = "healthcheck-port"
	AnnotationSuffixHealthCheckPath                    = "healthcheck-path"
	AnnotationSuffixHealthCheckIntervalSeconds         = "healthcheck-interval-seconds"
	AnnotationSuffixHealthCheckTimeoutSeconds          = "healthcheck-timeout-seconds"
	AnnotationSuffixHealthCheckHealthyThresholdCount   = "healthy-threshold-count"
	AnnotationSuffixHealthCheckUnhealthyThresholdCount = "unhealthy-threshold-count"
	AnnotationSuffixHealthCheckSuccessCodes            = "success-codes"
)

type AnnotationParser interface {
	ParseStringAnnotation(annotation string, value *string, annotations ...map[string]string) bool
	ParseInt64Annotation(annotation string, value *int64, annotations ...map[string]string) (bool, error)
	ParseStringSliceAnnotation(annotation string, value *[]string, annotations ...map[string]string) bool
	ParseJSONAnnotation(annotation string, value interface{}, annotations ...map[string]string) (bool, error)
}

// NewSuffixAnnotationParser constructs an new AnnotationParser that parse annotation with specific prefix..
func NewSuffixAnnotationParser(annotationPrefix string) AnnotationParser {
	return &suffixAnnotationParser{
		annotationPrefix: annotationPrefix,
	}
}

type suffixAnnotationParser struct {
	annotationPrefix string
}

func (p *suffixAnnotationParser) ParseStringAnnotation(suffix string, value *string, annotations ...map[string]string) bool {
	key := p.buildAnnotationKey(suffix)
	raw, ok := algorithm.MapFindFirst(key, annotations...)
	if !ok {
		return false
	}
	*value = raw
	return true
}

func (p *suffixAnnotationParser) ParseInt64Annotation(suffix string, value *int64, annotations ...map[string]string) (bool, error) {
	key := p.buildAnnotationKey(suffix)
	raw, ok := algorithm.MapFindFirst(key, annotations...)
	if !ok {
		return false, nil
	}
	i, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return true, errors.Wrapf(err, "failed to parse annotation, %v: %v", key, raw)
	}
	*value = i
	return true, nil
}

func (p *suffixAnnotationParser) ParseStringSliceAnnotation(suffix string, value *[]string, annotations ...map[string]string) bool {
	key := p.buildAnnotationKey(suffix)
	raw, ok := algorithm.MapFindFirst(key, annotations...)
	if !ok {
		return false
	}

	var result []string
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		result = append(result, part)
	}
	*value = result
	return true
}

func (p *suffixAnnotationParser) ParseJSONAnnotation(suffix string, value interface{}, annotations ...map[string]string) (bool, error) {
	key := p.buildAnnotationKey(suffix)
	raw, ok := algorithm.MapFindFirst(key, annotations...)
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return true, errors.Wrapf(err, "failed to parse annotation, %v: %v", key, raw)
	}
	return true, nil
}

func (p *suffixAnnotationParser) buildAnnotationKey(suffix string) string {
	return fmt.Sprintf("%v/%v", p.annotationPrefix, suffix)
}
