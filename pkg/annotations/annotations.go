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
	albec2 "github.com/coreos/alb-ingress-controller/pkg/aws/ec2"
	albprom "github.com/coreos/alb-ingress-controller/pkg/prometheus"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
	"github.com/karlseguin/ccache"
	"github.com/prometheus/client_golang/prometheus"
)

var cache = ccache.New(ccache.Configure())

const (
	backendProtocolKey            = "alb.ingress.kubernetes.io/backend-protocol"
	certificateArnKey             = "alb.ingress.kubernetes.io/certificate-arn"
	connectionIdleTimeoutKey      = "alb.ingress.kubernetes.io/connection-idle-timeout"
	healthcheckIntervalSecondsKey = "alb.ingress.kubernetes.io/healthcheck-interval-seconds"
	healthcheckPathKey            = "alb.ingress.kubernetes.io/healthcheck-path"
	healthcheckPortKey            = "alb.ingress.kubernetes.io/healthcheck-port"
	healthcheckProtocolKey        = "alb.ingress.kubernetes.io/healthcheck-protocol"
	healthcheckTimeoutSecondsKey  = "alb.ingress.kubernetes.io/healthcheck-timeout-seconds"
	healthyThresholdCountKey      = "alb.ingress.kubernetes.io/healthy-threshold-count"
	unhealthyThresholdCountKey    = "alb.ingress.kubernetes.io/unhealthy-threshold-count"
	portKey                       = "alb.ingress.kubernetes.io/listen-ports"
	schemeKey                     = "alb.ingress.kubernetes.io/scheme"
	securityGroupsKey             = "alb.ingress.kubernetes.io/security-groups"
	subnetsKey                    = "alb.ingress.kubernetes.io/subnets"
	successCodesKey               = "alb.ingress.kubernetes.io/successCodes"
	tagsKey                       = "alb.ingress.kubernetes.io/tags"
	clusterTagKey                 = "tag:kubernetes.io/cluster"
	clusterTagValue               = "shared"
	albRoleTagKey                 = "tag:kubernetes.io/role/alb-ingress"
	albManagedSubnetsCacheKey     = "alb-managed-subnets"
)

// Annotations contains all of the annotation configuration for an ingress
type Annotations struct {
	BackendProtocol            *string
	CertificateArn             *string
	ConnectionIdleTimeout      int64
	HealthcheckIntervalSeconds *int64
	HealthcheckPath            *string
	HealthcheckPort            *string
	HealthcheckProtocol        *string
	HealthcheckTimeoutSeconds  *int64
	HealthyThresholdCount      *int64
	UnhealthyThresholdCount    *int64
	Ports                      []PortData
	Scheme                     *string
	SecurityGroups             util.AWSStringSlice
	Subnets                    util.Subnets
	SuccessCodes               *string
	Tags                       []*elbv2.Tag
	VPCID                      *string
}

type PortData struct {
	Port   int64
	Scheme string
}

// ParseAnnotations validates and loads all the annotations provided into the Annotations struct.
// If there is an issue with an annotation, an error is returned. In the case of an error, the
// annotations are also cached, meaning there will be no reattempt to parse annotations until the
// cache expires or the value(s) change.
func ParseAnnotations(annotations map[string]string, clusterName string) (*Annotations, error) {
	if annotations == nil {
		return nil, fmt.Errorf("Necessary annotations missing. Must include at least %s, %s, %s", subnetsKey, securityGroupsKey, schemeKey)
	}

	sortedAnnotations := util.SortedMap(annotations)
	cacheKey := "annotations " + log.Prettify(sortedAnnotations)

	if badAnnotations := cacheLookup(cacheKey); badAnnotations != nil {
		return nil, fmt.Errorf("%v (cache hit)", badAnnotations.Value().(error).Error())
	}

	a := new(Annotations)
	for _, err := range []error{
		a.setBackendProtocol(annotations),
		a.setConnectionIdleTimeout(annotations),
		a.setCertificateArn(annotations),
		a.setHealthcheckIntervalSeconds(annotations),
		a.setHealthcheckPath(annotations),
		a.setHealthcheckPort(annotations),
		a.setHealthcheckProtocol(annotations),
		a.setHealthcheckTimeoutSeconds(annotations),
		a.setHealthyThresholdCount(annotations),
		a.setUnhealthyThresholdCount(annotations),
		a.setPorts(annotations),
		a.setScheme(annotations),
		a.setSecurityGroups(annotations),
		a.setSubnets(annotations, clusterName),
		a.setSuccessCodes(annotations),
		a.setTags(annotations),
	} {
		if err != nil {
			cache.Set(cacheKey, err, 1*time.Hour)
			return nil, err
		}
	}
	return a, nil
}

func (a *Annotations) setBackendProtocol(annotations map[string]string) error {
	if annotations[backendProtocolKey] == "" {
		a.BackendProtocol = aws.String("HTTP")
	} else {
		a.BackendProtocol = aws.String(annotations[backendProtocolKey])
	}
	return nil
}

func (a *Annotations) setCertificateArn(annotations map[string]string) error {
	if cert, ok := annotations[certificateArnKey]; ok {
		a.CertificateArn = aws.String(cert)
		if c := cacheLookup(cert); c == nil || c.Expired() {
			if err := a.validateCertARN(); err != nil {
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
	a.ConnectionIdleTimeout = i
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

func (a *Annotations) setScheme(annotations map[string]string) error {
	switch {
	case annotations[schemeKey] == "":
		return fmt.Errorf(`Necessary annotations missing. Must include %s`, schemeKey)
	case annotations[schemeKey] != "internal" && annotations[schemeKey] != "internet-facing":
		return fmt.Errorf("ALB Scheme [%v] must be either `internal` or `internet-facing`", annotations[schemeKey])
	}
	a.Scheme = aws.String(annotations[schemeKey])
	return nil
}

func (a *Annotations) setSecurityGroups(annotations map[string]string) error {
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
		in := &ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{{
			Name:   aws.String("tag:Name"),
			Values: names,
		}}}

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

	if c := cacheLookup(*a.SecurityGroups.Hash()); c == nil || c.Expired() {
		if err := a.validateSecurityGroups(); err != nil {
			return err
		}
		cache.Set(*a.SecurityGroups.Hash(), "success", 30*time.Minute)
	}

	return nil
}

func (a *Annotations) setSubnets(annotations map[string]string, clusterName string) error {
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
		in := &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{{
			Name:   aws.String("tag:Name"),
			Values: names,
		}}}

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
		if err := a.resolveVPCValidateSubnets(); err != nil {
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
	if annotations[successCodesKey] == "" {
		a.SuccessCodes = aws.String("200")
	} else {
		a.SuccessCodes = aws.String(annotations[successCodesKey])
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

func cacheLookup(key string) *ccache.Item {
	i := cache.Get(key)
	if i == nil || i.Expired() {
		return nil
	}
	return i
}
