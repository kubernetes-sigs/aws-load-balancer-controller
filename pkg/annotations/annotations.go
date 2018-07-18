package annotations

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/karlseguin/ccache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/config"
	albprom "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/prometheus"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	"github.com/prometheus/client_golang/prometheus"
)

var cache = ccache.New(ccache.Configure())

const (
	backendProtocolKey            = "alb.ingress.kubernetes.io/backend-protocol"
	certificateArnKey             = "alb.ingress.kubernetes.io/certificate-arn"
	connectionIdleTimeoutKey      = "alb.ingress.kubernetes.io/connection-idle-timeout"
	wafACLIDKey                   = "alb.ingress.kubernetes.io/waf-acl-id"
	healthcheckIntervalSecondsKey = "alb.ingress.kubernetes.io/healthcheck-interval-seconds"
	healthcheckPathKey            = "alb.ingress.kubernetes.io/healthcheck-path"
	healthcheckPortKey            = "alb.ingress.kubernetes.io/healthcheck-port"
	healthcheckProtocolKey        = "alb.ingress.kubernetes.io/healthcheck-protocol"
	healthcheckTimeoutSecondsKey  = "alb.ingress.kubernetes.io/healthcheck-timeout-seconds"
	healthyThresholdCountKey      = "alb.ingress.kubernetes.io/healthy-threshold-count"
	unhealthyThresholdCountKey    = "alb.ingress.kubernetes.io/unhealthy-threshold-count"
	inboundCidrsKey               = "alb.ingress.kubernetes.io/security-group-inbound-cidrs"
	loadbalancerAttributesKey     = "alb.ingress.kubernetes.io/load-balancer-attributes"
	loadbalancerAttributesAltKey  = "alb.ingress.kubernetes.io/attributes"
	portKey                       = "alb.ingress.kubernetes.io/listen-ports"
	schemeKey                     = "alb.ingress.kubernetes.io/scheme"
	sslPolicyKey                  = "alb.ingress.kubernetes.io/ssl-policy"
	ipAddressTypeKey              = "alb.ingress.kubernetes.io/ip-address-type"
	securityGroupsKey             = "alb.ingress.kubernetes.io/security-groups"
	subnetsKey                    = "alb.ingress.kubernetes.io/subnets"
	successCodesKey               = "alb.ingress.kubernetes.io/success-codes"
	successCodesAltKey            = "alb.ingress.kubernetes.io/successCodes"
	tagsKey                       = "alb.ingress.kubernetes.io/tags"
	ignoreHostHeader              = "alb.ingress.kubernetes.io/ignore-host-header"
	targetGroupAttributesKey      = "alb.ingress.kubernetes.io/target-group-attributes"
	clusterTagKey                 = "tag:kubernetes.io/cluster"
	clusterTagValue               = "shared"
	albRoleTagKey                 = "tag:kubernetes.io/role/alb-ingress"
	albManagedSubnetsCacheKey     = "alb-managed-subnets"
)

// Annotations contains all of the annotation configuration for an ingress
type Annotations struct {
	BackendProtocol            *string
	CertificateArn             *string
	ConnectionIdleTimeout      *int64
	WafACLID                   *string
	HealthcheckIntervalSeconds *int64
	HealthcheckPath            *string
	HealthcheckPort            *string
	HealthcheckProtocol        *string
	HealthcheckTimeoutSeconds  *int64
	HealthyThresholdCount      *int64
	UnhealthyThresholdCount    *int64
	InboundCidrs               util.Cidrs
	Ports                      []PortData
	Scheme                     *string
	IPAddressType              *string
	SecurityGroups             util.AWSStringSlice
	Subnets                    util.Subnets
	SuccessCodes               *string
	Tags                       []*elbv2.Tag
	IgnoreHostHeader           *bool
	TargetGroupAttributes      albelbv2.TargetGroupAttributes
	SslPolicy                  *string
	VPCID                      *string
	LoadBalancerAttributes     []*elbv2.LoadBalancerAttribute
}

type PortData struct {
	Port   int64
	Scheme string
}

type AnnotationFactory interface {
	ParseAnnotations(*ParseAnnotationsOptions) (*Annotations, error)
}

type ValidatingAnnotationFactory struct {
	validator   Validator
	clusterName string
}

type NewValidatingAnnotationFactoryOptions struct {
	Validator   Validator
	ClusterName string
}

func NewValidatingAnnotationFactory(opts *NewValidatingAnnotationFactoryOptions) *ValidatingAnnotationFactory {
	return &ValidatingAnnotationFactory{
		validator:   opts.Validator,
		clusterName: opts.ClusterName,
	}
}

type ParseAnnotationsOptions struct {
	Annotations map[string]string
	Namespace   string
	IngressName string
	ServiceName string
}

// ParseAnnotations validates and loads all the annotations provided into the Annotations struct.
// If there is an issue with an annotation, an error is returned. In the case of an error, the
// annotations are also cached, meaning there will be no reattempt to parse annotations until the
// cache expires or the value(s) change.
func (vf *ValidatingAnnotationFactory) ParseAnnotations(opts *ParseAnnotationsOptions) (*Annotations, error) {
	annotations := opts.Annotations

	if annotations == nil {
		return nil, fmt.Errorf("Necessary annotations missing. Must include at least %s, %s, %s", subnetsKey, securityGroupsKey, schemeKey)
	}

	sortedAnnotations := util.SortedMap(annotations)
	cacheKey := fmt.Sprintf("annotations:%v:%v:%v:%v", opts.Namespace, opts.IngressName, opts.ServiceName, log.Prettify(sortedAnnotations))

	if badAnnotations := cacheLookup(cacheKey); badAnnotations != nil {
		return nil, fmt.Errorf("%v (cache hit)", badAnnotations.Value().(error).Error())
	}

	a := new(Annotations)
	for _, err := range []error{
		a.setBackendProtocol(annotations),
		a.setConnectionIdleTimeout(annotations),
		a.setCertificateArn(annotations, vf.validator),
		a.setHealthcheckIntervalSeconds(annotations),
		a.setHealthcheckPath(annotations),
		a.setHealthcheckPort(annotations),
		a.setHealthcheckProtocol(annotations),
		a.setHealthcheckTimeoutSeconds(annotations),
		a.setHealthyThresholdCount(annotations),
		a.setUnhealthyThresholdCount(annotations),
		a.setInboundCidrs(annotations, vf.validator),
		a.setPorts(annotations),
		a.setScheme(annotations, opts.Namespace, opts.IngressName, vf.validator),
		a.setIPAddressType(annotations),
		a.setSecurityGroups(annotations, vf.validator),
		a.setSubnets(annotations, vf.clusterName, vf.validator),
		a.setSuccessCodes(annotations),
		a.setTags(annotations),
		a.setIgnoreHostHeader(annotations),
		a.setWafACLID(annotations, vf.validator),
		a.setLoadBalancerAttributes(annotations),
		a.setTargetGroupAttributes(annotations),
		a.setSslPolicy(annotations, vf.validator),
	} {
		if err != nil {
			cache.Set(cacheKey, err, 1*time.Hour)
			return nil, err
		}
	}
	return a, nil
}

func (a *Annotations) setLoadBalancerAttributes(annotations map[string]string) error {
	var attrs []*elbv2.LoadBalancerAttribute
	var badAttrs []string
	v, ok := annotations[loadbalancerAttributesKey]
	if !ok {
		v = annotations[loadbalancerAttributesAltKey]
	}

	rawAttrs := util.NewAWSStringSlice(v)

	for _, rawAttr := range rawAttrs {
		parts := strings.Split(*rawAttr, "=")
		switch {
		case *rawAttr == "":
			continue
		case len(parts) != 2:
			badAttrs = append(badAttrs, *rawAttr)
			continue
		}
		attrs = append(attrs, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(parts[0]),
			Value: aws.String(parts[1]),
		})
	}
	a.LoadBalancerAttributes = attrs

	if len(badAttrs) > 0 {
		return fmt.Errorf("Unable to parse `%s` into Key=Value pair(s)", strings.Join(badAttrs, ", "))
	}
	return nil
}

func (a *Annotations) setBackendProtocol(annotations map[string]string) error {
	if annotations[backendProtocolKey] == "" {
		a.BackendProtocol = aws.String("HTTP")
	} else {
		a.BackendProtocol = aws.String(annotations[backendProtocolKey])
	}
	return nil
}

func (a *Annotations) setCertificateArn(annotations map[string]string, validator Validator) error {
	if cert, ok := annotations[certificateArnKey]; ok {
		a.CertificateArn = aws.String(cert)
		if c := cacheLookup(cert); c == nil || c.Expired() {
			if err := validator.ValidateCertARN(a); err != nil {
				return err
			}
			cache.Set(cert, "success", 30*time.Minute)
		}
	}
	return nil
}

func (a *Annotations) setConnectionIdleTimeout(annotations map[string]string) error {
	i, err := strconv.ParseInt(annotations[connectionIdleTimeoutKey], 10, 64)
	if err != nil {
		if annotations[connectionIdleTimeoutKey] != "" {
			return err
		}
		return nil
	}
	// aws only accepts a range of 1-3600 seconds
	if i < 1 || i > 3600 {
		return fmt.Errorf("Invalid connection idle timeout provided must be within 1-3600 seconds. Was: %d", i)
	}
	a.ConnectionIdleTimeout = aws.Int64(i)
	return nil
}

func (a *Annotations) setHealthcheckIntervalSeconds(annotations map[string]string) error {
	i, err := strconv.ParseInt(annotations[healthcheckIntervalSecondsKey], 10, 64)
	if err != nil {
		if annotations[healthcheckIntervalSecondsKey] != "" {
			return err
		}
		a.HealthcheckIntervalSeconds = aws.Int64(15)
		return nil
	}
	a.HealthcheckIntervalSeconds = &i
	return nil
}

func (a *Annotations) setHealthcheckPath(annotations map[string]string) error {
	switch {
	case annotations[healthcheckPathKey] == "":
		a.HealthcheckPath = aws.String("/")
		return nil
	}
	a.HealthcheckPath = aws.String(annotations[healthcheckPathKey])
	return nil
}

func (a *Annotations) setHealthcheckPort(annotations map[string]string) error {
	switch {
	case annotations[healthcheckPortKey] == "":
		a.HealthcheckPort = aws.String("traffic-port")
		return nil
	}
	a.HealthcheckPort = aws.String(annotations[healthcheckPortKey])
	return nil
}

func (a *Annotations) setHealthcheckProtocol(annotations map[string]string) error {
	if annotations[healthcheckProtocolKey] != "" {
		a.HealthcheckProtocol = aws.String(annotations[healthcheckProtocolKey])
	}
	return nil
}

func (a *Annotations) setHealthcheckTimeoutSeconds(annotations map[string]string) error {
	i, err := strconv.ParseInt(annotations[healthcheckTimeoutSecondsKey], 10, 64)
	if err != nil {
		if annotations[healthcheckTimeoutSecondsKey] != "" {
			return err
		}
		a.HealthcheckTimeoutSeconds = aws.Int64(5)
		return nil
	}
	// If interval is set at our above timeout, AWS will reject targetgroup creation
	if i >= *a.HealthcheckIntervalSeconds {
		return fmt.Errorf("Healthcheck timeout must be less than healthcheck interval. Timeout was: %d. Interval was %d.",
			i, *a.HealthcheckIntervalSeconds)
	}
	a.HealthcheckTimeoutSeconds = &i
	return nil
}

func (a *Annotations) setHealthyThresholdCount(annotations map[string]string) error {
	i, err := strconv.ParseInt(annotations[healthyThresholdCountKey], 10, 64)
	if err != nil {
		if annotations[healthyThresholdCountKey] != "" {
			return err
		}
		a.HealthyThresholdCount = aws.Int64(2)
		return nil
	}
	a.HealthyThresholdCount = &i
	return nil
}

func (a *Annotations) setUnhealthyThresholdCount(annotations map[string]string) error {
	i, err := strconv.ParseInt(annotations[unhealthyThresholdCountKey], 10, 64)
	if err != nil {
		if annotations[unhealthyThresholdCountKey] != "" {
			return err
		}
		a.UnhealthyThresholdCount = aws.Int64(2)
		return nil
	}
	a.UnhealthyThresholdCount = &i
	return nil
}

// parsePorts takes a JSON array describing what ports and protocols should be used. When the JSON
// is empty, implying the annotation was not present, desired ports are set to the default. The
// default port value is 80 when a certArn is not present and 443 when it is.
func (a *Annotations) setPorts(annotations map[string]string) error {
	lps := []PortData{}
	// If port data is empty, default to port 80 or 443 contingent on whether a certArn was specified.
	if annotations[portKey] == "" {
		switch annotations[certificateArnKey] {
		case "":
			lps = append(lps, PortData{int64(80), "HTTP"})
		default:
			lps = append(lps, PortData{int64(443), "HTTPS"})
		}
		a.Ports = lps
		return nil
	}

	// Container to hold json in structured format after unmarshaling.
	c := []map[string]int64{}
	err := json.Unmarshal([]byte(annotations[portKey]), &c)
	if err != nil {
		return fmt.Errorf("%s JSON structure was invalid. %s", portKey, err.Error())
	}

	// Iterate over listeners in list. Validate port and protcol are correct, then inject them into
	// the list of ListenerPorts.
	for _, l := range c {
		for k, v := range l {
			// Verify port value is valid for ALB.
			// ALBS (from AWS): Ports need to be a number between 1 and 65535
			if v < 1 || v > 65535 {
				return fmt.Errorf("Invalid port provided. Must be between 1 and 65535. It was %d", v)
			}
			switch {
			case k == "HTTP":
				lps = append(lps, PortData{v, k})
			case k == "HTTPS":
				lps = append(lps, PortData{v, k})
			default:
				return fmt.Errorf("Invalid protocol provided. Must be HTTP or HTTPS and in order to use HTTPS you must have specified a certificate ARN")
			}
		}
	}

	a.Ports = lps
	return nil
}

func (a *Annotations) setInboundCidrs(annotations map[string]string, validator Validator) error {
	for _, inboundCidr := range util.NewAWSStringSlice(annotations[inboundCidrsKey]) {
		a.InboundCidrs = append(a.InboundCidrs, inboundCidr)
		if err := validator.ValidateInboundCidrs(a); err != nil {
			return err
		}
	}

	return nil
}

func (a *Annotations) setScheme(annotations map[string]string, ingressNamespace, ingressName string, validator Validator) error {
	switch {
	case annotations[schemeKey] == "":
		return fmt.Errorf(`Necessary annotations missing. Must include %s`, schemeKey)
	case annotations[schemeKey] != "internal" && annotations[schemeKey] != "internet-facing":
		return fmt.Errorf("ALB Scheme [%v] must be either `internal` or `internet-facing`", annotations[schemeKey])
	case annotations[schemeKey] == "internal":
		return nil
	}
	cacheKey := fmt.Sprintf(
		"scheme-%v-%s-%s-%s",
		config.RestrictScheme,
		config.RestrictSchemeNamespace,
		ingressNamespace,
		ingressName,
	)
	// Since we only cache successes here, if the item is in the cache, then there should be no error
	if item := cacheLookup(cacheKey); item != nil {
		a.Scheme = aws.String(annotations[schemeKey])
		return nil
	}
	isValid := validator.ValidateScheme(annotations[schemeKey], ingressNamespace, ingressName)
	if !isValid {
		return fmt.Errorf("ALB scheme internet-facing not permitted for namespace/ingress: %s/%s", ingressNamespace, ingressName)
	}
	a.Scheme = aws.String(annotations[schemeKey])
	// only cache successes.
	// failures, returned as errors, will be cached up the stack in ParseAnnotations, the caller of this func.
	cache.Set(cacheKey, isValid, time.Minute*10)
	return nil
}

func (a *Annotations) setIPAddressType(annotations map[string]string) error {
	switch {
	case annotations[ipAddressTypeKey] == "":
		a.IPAddressType = aws.String("ipv4")
		return nil
	case annotations[ipAddressTypeKey] != "ipv4" && annotations[ipAddressTypeKey] != "dualstack":
		return fmt.Errorf("ALB IP Address Type [%v] must be either `ipv4` or `dualstack`", annotations[ipAddressTypeKey])
	}
	a.IPAddressType = aws.String(annotations[ipAddressTypeKey])
	return nil
}

func (a *Annotations) setSecurityGroups(annotations map[string]string, validator Validator) error {
	// no security groups specified means controller should manage them, if so return and sg will be
	// created and managed during reconcile.
	if _, ok := annotations[securityGroupsKey]; !ok {
		return nil
	}
	var names []*string

	for _, sg := range util.NewAWSStringSlice(annotations[securityGroupsKey]) {
		if strings.HasPrefix(*sg, "sg-") {
			a.SecurityGroups = append(a.SecurityGroups, sg)
			continue
		}

		item := cacheLookup(*sg)
		if item != nil {
			for i := range item.Value().([]string) {
				albprom.AWSCache.With(prometheus.Labels{"cache": "securitygroups", "action": "hit"}).Add(float64(1))
				a.SecurityGroups = append(a.SecurityGroups, &item.Value().([]string)[i])
			}
			continue
		}

		albprom.AWSCache.With(prometheus.Labels{"cache": "securitygroups", "action": "miss"}).Add(float64(1))
		names = append(names, sg)
	}

	if len(names) > 0 {
		var vpcIds []*string
		vpcId, err := albec2.EC2svc.GetVPCID()
		if err != nil {
			return err
		}
		vpcIds = append(vpcIds, vpcId)
		in := &ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: names,
			},
			{
				Name:   aws.String("vpc-id"),
				Values: vpcIds,
			},
		}}

		describeSecurityGroupsOutput, err := albec2.EC2svc.DescribeSecurityGroups(in)
		if err != nil {
			return fmt.Errorf("Unable to fetch security groups %v: %v", in.Filters, err)
		}

		for _, sg := range describeSecurityGroupsOutput.SecurityGroups {
			value, ok := util.EC2Tags(sg.Tags).Get("Name")
			if ok {
				if item := cacheLookup(value); item != nil {
					nv := append(item.Value().([]string), *sg.GroupId)
					cache.Set(value, nv, time.Minute*60)
				} else {
					sgIds := []string{*sg.GroupId}
					cache.Set(value, sgIds, time.Minute*60)
				}
				a.SecurityGroups = append(a.SecurityGroups, sg.GroupId)
			}
		}
	}

	sort.Sort(a.SecurityGroups)
	if len(a.SecurityGroups) == 0 {
		return fmt.Errorf("unable to resolve any security groups from annotation containing: [%s]", annotations[securityGroupsKey])
	}

	if c := cacheLookup(a.SecurityGroups.Hash()); c == nil || c.Expired() {
		if err := validator.ValidateSecurityGroups(a); err != nil {
			return err
		}
		cache.Set(a.SecurityGroups.Hash(), "success", 30*time.Minute)
	}

	return nil
}

func (a *Annotations) setSubnets(annotations map[string]string, clusterName string, validator Validator) error {
	var names []*string
	var out util.AWSStringSlice

	// if the subnet annotation isn't specified, lookup appropriate subnets to use
	if annotations[subnetsKey] == "" {

		// check to see if subnets already exist in cache, if so return those
		item := cacheLookup(albManagedSubnetsCacheKey)
		if item != nil {
			albprom.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "hit"}).Add(float64(1))
			a.Subnets = item.Value().(util.Subnets)
			return nil
		}
		albprom.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "miss"}).Add(float64(1))

		in := &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
			{
				Name:   aws.String(fmt.Sprintf("%s/%s", clusterTagKey, clusterName)),
				Values: []*string{aws.String(clusterTagValue)},
			},
			{
				Name:   aws.String(albRoleTagKey),
				Values: []*string{},
			},
		}}
		o, err := albec2.EC2svc.DescribeSubnets(in)
		if err != nil {
			return fmt.Errorf("Unable to fetch subnets %v: %v", in.Filters, err)
		}

		useableSubnets := []*ec2.Subnet{}
		for _, subnet := range o.Subnets {
			if subnetIsUsable(subnet, useableSubnets) {
				useableSubnets = append(useableSubnets, subnet)
				out = append(out, subnet.SubnetId)
			}
		}

		if len(useableSubnets) < 2 {
			return fmt.Errorf("Retrieval of subnets failed to resolve 2 qualified subnets. Subnets must "+
				"contain the %s/%s tag with a value of %s and the %s tag signifying it should be used for ALBs "+
				"Additionally, there must be at least 2 subnets with unique availability zones as required by "+
				"ALBs. Either tag subnets to meet this requirement or use the subnets annotation on the "+
				"ingress resource to explicitly call out what subnets to use for ALB creation. The subnets "+
				"that did resolve were %v.", clusterTagKey, clusterName, clusterTagValue, albRoleTagKey,
				awsutil.Prettify(useableSubnets))
		}
		sort.Sort(out)
		a.Subnets = util.Subnets(out)
		cache.Set(albManagedSubnetsCacheKey, a.Subnets, time.Minute*60)
		return nil
	}

	for _, subnet := range util.NewAWSStringSlice(annotations[subnetsKey]) {
		if strings.HasPrefix(*subnet, "subnet-") {
			out = append(out, subnet)
			continue
		}

		item := cacheLookup(*subnet)
		if item != nil {
			for i := range item.Value().([]string) {
				albprom.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "hit"}).Add(float64(1))
				out = append(out, &item.Value().([]string)[i])
			}
			continue
		}
		albprom.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "miss"}).Add(float64(1))

		names = append(names, subnet)
	}

	if len(names) > 0 {
		var vpcIds []*string
		vpcId, err := albec2.EC2svc.GetVPCID()
		if err != nil {
			return err
		}
		vpcIds = append(vpcIds, vpcId)
		in := &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: names,
			},
			{
				Name:   aws.String("vpc-id"),
				Values: vpcIds,
			},
		}}

		describeSubnetsOutput, err := albec2.EC2svc.DescribeSubnets(in)
		if err != nil {
			return fmt.Errorf("Unable to fetch subnets %v: %v", in.Filters, err)
		}

		for _, subnet := range describeSubnetsOutput.Subnets {
			value, ok := util.EC2Tags(subnet.Tags).Get("Name")
			if ok {
				if item := cacheLookup(value); item != nil {
					nv := append(item.Value().([]string), *subnet.SubnetId)
					cache.Set(value, nv, time.Minute*60)
				} else {
					subnetIds := []string{*subnet.SubnetId}
					cache.Set(value, subnetIds, time.Minute*60)
				}
				out = append(out, subnet.SubnetId)
			}
		}
	}

	sort.Sort(out)
	if len(out) == 0 {
		return fmt.Errorf("unable to resolve any subnets from: %s", annotations[subnetsKey])
	}

	a.Subnets = util.Subnets(out)

	// Validate subnets
	if c := cacheLookup(a.Subnets.String()); c == nil || c.Expired() {
		if err := validator.ResolveVPCValidateSubnets(a); err != nil {
			return err
		}
		cache.Set(a.Subnets.String(), "success", 30*time.Minute)
	}

	return nil
}

// subnetIsUsable determines if the subnet shares the same availablity zone as a subnet in the
// existing list. If it does, false is returned as you cannot have albs provisioned to 2 subnets in
// the same availability zone.
func subnetIsUsable(new *ec2.Subnet, existing []*ec2.Subnet) bool {
	for _, subnet := range existing {
		if *new.AvailabilityZone == *subnet.AvailabilityZone {
			return false
		}
	}
	return true
}

func (a *Annotations) setSuccessCodes(annotations map[string]string) error {
	key := successCodesKey
	if annotations[successCodesKey] == "" && annotations[successCodesAltKey] != "" {
		key = successCodesAltKey
	}
	if annotations[key] == "" {
		a.SuccessCodes = aws.String("200")
	} else {
		a.SuccessCodes = aws.String(annotations[key])
	}
	return nil
}

func (a *Annotations) setTags(annotations map[string]string) error {
	var tags []*elbv2.Tag
	var badTags []string
	rawTags := util.NewAWSStringSlice(annotations[tagsKey])

	for _, rawTag := range rawTags {
		parts := strings.Split(*rawTag, "=")
		switch {
		case *rawTag == "":
			continue
		case len(parts) < 2:
			badTags = append(badTags, *rawTag)
			continue
		}
		tags = append(tags, &elbv2.Tag{
			Key:   aws.String(parts[0]),
			Value: aws.String(parts[1]),
		})
	}
	a.Tags = tags

	if len(badTags) > 0 {
		return fmt.Errorf("Unable to parse `%s` into Key=Value pair(s)", strings.Join(badTags, ", "))
	}
	return nil
}
func (a *Annotations) setTargetGroupAttributes(annotations map[string]string) error {
	var badAttrs []string
	rawAttrs := util.NewAWSStringSlice(annotations[targetGroupAttributesKey])

	a.TargetGroupAttributes.Set("deregistration_delay.timeout_seconds", "300")
	a.TargetGroupAttributes.Set("slow_start.duration_seconds", "0")
	a.TargetGroupAttributes.Set("stickiness.enabled", "false")
	a.TargetGroupAttributes.Set("stickiness.lb_cookie.duration_seconds", "86400")
	a.TargetGroupAttributes.Set("stickiness.type", "lb_cookie")

	for _, rawAttr := range rawAttrs {
		parts := strings.Split(*rawAttr, "=")
		switch {
		case *rawAttr == "":
			continue
		case len(parts) != 2:
			badAttrs = append(badAttrs, *rawAttr)
			continue
		}
		a.TargetGroupAttributes.Set(parts[0], parts[1])
	}

	if len(badAttrs) > 0 {
		return fmt.Errorf("Unable to parse `%s` into Key=Value pair(s)", strings.Join(badAttrs, ", "))
	}

	return nil
}

func (a *Annotations) setIgnoreHostHeader(annotations map[string]string) error {
	if ihh, err := strconv.ParseBool(annotations[ignoreHostHeader]); err == nil {
		a.IgnoreHostHeader = aws.Bool(ihh)
	} else {
		a.IgnoreHostHeader = aws.Bool(false)
	}
	return nil
}

func (a *Annotations) setWafACLID(annotations map[string]string, validator Validator) error {
	if wafACLID, ok := annotations[wafACLIDKey]; ok {
		a.WafACLID = aws.String(wafACLID)
		if c := cacheLookup(wafACLID); c == nil || c.Expired() {
			if err := validator.ValidateWafACLID(a); err != nil {
				cache.Set(wafACLID, "error", 1*time.Hour)
				return err
			}
			cache.Set(wafACLID, "success", 30*time.Minute)
		}
	}
	return nil
}

func (a *Annotations) setSslPolicy(annotations map[string]string, validator Validator) error {
	if a.CertificateArn != nil {
		a.SslPolicy = aws.String("ELBSecurityPolicy-2016-08") // AWS default policy
	}

	if sslPolicy, ok := annotations[sslPolicyKey]; ok {
		a.SslPolicy = aws.String(sslPolicy)
		if c := cacheLookup(sslPolicy); c == nil || c.Expired() {
			if err := validator.ValidateSslPolicy(a); err != nil {
				return err
			}
			cache.Set(sslPolicy, "success", 30*time.Minute)
		}
	}
	return nil
}

func cacheLookup(key string) *ccache.Item {
	i := cache.Get(key)
	if i == nil || i.Expired() {
		return nil
	}
	return i
}
