package annotations

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/karlseguin/ccache"
	albprom "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/prometheus"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	"github.com/prometheus/client_golang/prometheus"
)

var cache = ccache.New(ccache.Configure())

const (
	backendProtocolKey = "alb.ingress.kubernetes.io/backend-protocol"
	certificateArnKey  = "alb.ingress.kubernetes.io/certificate-arn"
	webACLIdKey        = "alb.ingress.kubernetes.io/web-acl-id"
	webACLIdAltKey     = "alb.ingress.kubernetes.io/waf-acl-id"

	healthcheckIntervalSecondsKey = "alb.ingress.kubernetes.io/healthcheck-interval-seconds"
	healthcheckPathKey            = "alb.ingress.kubernetes.io/healthcheck-path"
	healthcheckPortKey            = "alb.ingress.kubernetes.io/healthcheck-port"
	healthcheckProtocolKey        = "alb.ingress.kubernetes.io/healthcheck-protocol"
	healthcheckTimeoutSecondsKey  = "alb.ingress.kubernetes.io/healthcheck-timeout-seconds"

	healthyThresholdCountKey   = "alb.ingress.kubernetes.io/healthy-threshold-count"
	unhealthyThresholdCountKey = "alb.ingress.kubernetes.io/unhealthy-threshold-count"

	inboundCidrsKey              = "alb.ingress.kubernetes.io/security-group-inbound-cidrs"
	loadbalancerAttributesKey    = "alb.ingress.kubernetes.io/load-balancer-attributes"
	loadbalancerAttributesAltKey = "alb.ingress.kubernetes.io/attributes"
	portKey                      = "alb.ingress.kubernetes.io/listen-ports"
	schemeKey                    = "alb.ingress.kubernetes.io/scheme"
	sslPolicyKey                 = "alb.ingress.kubernetes.io/ssl-policy"
	ipAddressTypeKey             = "alb.ingress.kubernetes.io/ip-address-type"
	securityGroupsKey            = "alb.ingress.kubernetes.io/security-groups"
	subnetsKey                   = "alb.ingress.kubernetes.io/subnets"
	successCodesKey              = "alb.ingress.kubernetes.io/success-codes"
	successCodesAltKey           = "alb.ingress.kubernetes.io/successCodes"

	tagsKey = "alb.ingress.kubernetes.io/tags"

	ignoreHostHeader = "alb.ingress.kubernetes.io/ignore-host-header"

	targetTypeKey            = "alb.ingress.kubernetes.io/target-type"
	targetGroupAttributesKey = "alb.ingress.kubernetes.io/target-group-attributes"
)

// Annotations contains all of the annotation configuration for an ingress
type Annotations struct {
	BackendProtocol            *string
	CertificateArn             *string
	WebACLId                   *string
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
	TargetType                 *string
	SecurityGroups             util.AWSStringSlice
	Subnets                    util.Subnets
	SuccessCodes               *string
	Tags                       []*elbv2.Tag
	IgnoreHostHeader           *bool
	TargetGroupAttributes      albelbv2.TargetGroupAttributes
	SslPolicy                  *string
	VPCID                      *string
	LoadBalancerAttributes     albelbv2.LoadBalancerAttributes
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
	clusterName *string
}

type NewValidatingAnnotationFactoryOptions struct {
	Validator   Validator
	ClusterName *string
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
	Resources   *albrgt.Resources
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
		a.setTargetType(annotations),
		a.setSecurityGroups(annotations, vf.validator),
		a.setSubnets(annotations, *vf.clusterName, opts.Resources, vf.validator),
		a.setSuccessCodes(annotations),
		a.setTags(annotations),
		a.setIgnoreHostHeader(annotations),
		a.setWebACLId(annotations, vf.validator),
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

func (a *Annotations) setSubnets(annotations map[string]string, validator Validator) error {
	var out util.AWSStringSlice
	var names []*string

	// if the subnet annotation isn't specified, lookup appropriate subnets to use
	if annotations[subnetsKey] == "" {
		subnets, err := albec2.ClusterSubnets(a.Scheme)
		if err == nil {
			a.Subnets = subnets
		}
		return err
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
