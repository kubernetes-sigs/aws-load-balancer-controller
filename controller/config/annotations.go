package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/awsutil"
	"github.com/coreos/alb-ingress-controller/controller/util"
	"github.com/coreos/alb-ingress-controller/log"
	"github.com/golang/glog"
	"github.com/karlseguin/ccache"
	"github.com/prometheus/client_golang/prometheus"
)

var cache = ccache.New(ccache.Configure())

const (
	backendProtocolKey = "alb.ingress.kubernetes.io/backend-protocol"
	certificateArnKey  = "alb.ingress.kubernetes.io/certificate-arn"
	healthcheckPathKey = "alb.ingress.kubernetes.io/healthcheck-path"
	portKey            = "alb.ingress.kubernetes.io/listen-ports"
	schemeKey          = "alb.ingress.kubernetes.io/scheme"
	securityGroupsKey  = "alb.ingress.kubernetes.io/security-groups"
	subnetsKey         = "alb.ingress.kubernetes.io/subnets"
	successCodesKey    = "alb.ingress.kubernetes.io/successCodes"
	tagsKey            = "alb.ingress.kubernetes.io/Tags"
)

// Annotations contains all of the annotation configuration for an ingress
type Annotations struct {
	BackendProtocol *string
	CertificateArn  *string
	HealthcheckPath *string
	Ports           []ListenerPort
	Scheme          *string
	SecurityGroups  util.AWSStringSlice
	Subnets         util.Subnets
	SuccessCodes    *string
	Tags            []*elbv2.Tag
	VPCID           *string
}

// ListenerPort represents a listener defined in an ingress annotation. Specifically, it represents a
// port that an ALB should listen on along with the protocol (HTTP or HTTPS). When HTTPS, it's
// expected the certificate reprsented by Annotations.CertificateArn will be applied.
type ListenerPort struct {
	HTTPS bool
	Port  int64
}

// ParseAnnotations validates and loads all the annotations provided into the Annotations struct.
// If there is an issue with an annotation, an error is returned. In the case of an error, the
// annotations are also cached, meaning there will be no reattempt to parse annotations until the
// cache expires or the value(s) change.
func ParseAnnotations(annotations map[string]string) (*Annotations, error) {
	sortedAnnotations := util.SortedMap(annotations)
	cacheKey := "annotations " + awsutil.Prettify(sortedAnnotations)

	if badAnnotations := cache.Get(cacheKey); badAnnotations != nil {
		return nil, nil
	}

	// Verify required annotations present and are valid
	if annotations[successCodesKey] == "" {
		annotations[successCodesKey] = "200"
	}
	if annotations[backendProtocolKey] == "" {
		annotations[backendProtocolKey] = "HTTP"
	}
	if annotations[subnetsKey] == "" {
		cache.Set(cacheKey, "error", 1*time.Hour)
		return nil, fmt.Errorf(`Necessary annotations missing. Must include %s`, subnetsKey)
	}

	subnets, err := parseSubnets(annotations[subnetsKey])
	if err != nil {
		cache.Set(cacheKey, "error", 1*time.Hour)
		return nil, err
	}

	securitygroups, err := parseSecurityGroups(annotations[securityGroupsKey])
	if err != nil {
		cache.Set(cacheKey, "error", 1*time.Hour)
		return nil, err
	}
	scheme, err := parseScheme(annotations[schemeKey])
	if err != nil {
		cache.Set(cacheKey, "error", 1*time.Hour)
		return nil, err
	}

	ports, err := parsePorts(annotations[portKey], annotations[certificateArnKey])
	if err != nil {
		cache.Set(cacheKey, "error", 1*time.Hour)
		return nil, err
	}

	a := &Annotations{
		BackendProtocol: aws.String(annotations[backendProtocolKey]),
		Ports:           ports,
		Subnets:         subnets,
		Scheme:          scheme,
		SecurityGroups:  securitygroups,
		SuccessCodes:    aws.String(annotations[successCodesKey]),
		Tags:            stringToTags(annotations[tagsKey]),
		HealthcheckPath: parseHealthcheckPath(annotations[healthcheckPathKey]),
	}

	// Begin all validations needed to qualify the ingress resource.
	if cert, ok := annotations[certificateArnKey]; ok {
		a.CertificateArn = aws.String(cert)
		if err := a.validateCertARN(); err != nil {
			cache.Set(cacheKey, "error", 1*time.Hour)
			return nil, err
		}
	}
	if err := a.resolveVPCValidateSubnets(); err != nil {
		cache.Set(cacheKey, "error", 1*time.Hour)
		return nil, err
	}
	if err := a.validateSecurityGroups(); err != nil {
		cache.Set(cacheKey, "error", 1*time.Hour)
		return nil, err
	}

	return a, nil
}

// parsePorts takes a JSON array describing what ports and protocols should be used. When the JSON
// is empty, implying the annotation was not present, desired ports are set to the default. The
// default port value is 80 when a certArn is not present and 443 when it is.
func parsePorts(data, certArn string) ([]ListenerPort, error) {
	lps := []ListenerPort{}
	// If port data is empty, default to port 80 or 443 contingent on whether a certArn was specified.
	if data == "" {
		switch certArn {
		case "":
			lps = append(lps, ListenerPort{false, int64(80)})
		default:
			lps = append(lps, ListenerPort{true, int64(443)})
		}
		return lps, nil
	}

	// Container to hold json in structured format after unmarshaling.
	c := []map[string]int64{}
	err := json.Unmarshal([]byte(data), &c)
	if err != nil {
		return nil, fmt.Errorf("JSON structure was invalid. %s", err.Error())
	}

	// Iterate over listeners in list. Validate port and protcol are correct, then inject them into
	// the list of ListenerPorts.
	for _, l := range c {
		for k, v := range l {
			// Verify port value is valid for ALB.
			// ALBS (from AWS): Ports need to be a number between 1 and 65535
			if v < 1 || v > 65535 {
				return nil, fmt.Errorf("Invalid port provided. Must be between 1 and 65535. It was %d", v)
			}
			switch {
			case k == "HTTP":
				lps = append(lps, ListenerPort{false, v})
			case k == "HTTPS" && certArn != "":
				lps = append(lps, ListenerPort{true, v})
			default:
				return nil, fmt.Errorf("invalid protocol provided. Must be HTTP or HTTPS and in order to use HTTPS you must have specified a certtificate ARN")
			}
		}
	}

	return lps, nil
}

func parseHealthcheckPath(s string) *string {
	switch {
	case s == "":
		return aws.String("/")
	}
	return aws.String(s)
}

func parseScheme(s string) (*string, error) {
	switch {
	case s == "":
		return aws.String(""), fmt.Errorf(`Necessary annotations missing. Must include %s`, schemeKey)
	case s != "internal" && s != "internet-facing":
		return aws.String(""), fmt.Errorf("ALB Scheme [%v] must be either `internal` or `internet-facing`", s)
	}
	return aws.String(s), nil
}

func stringToAwsSlice(s string) (out []*string) {
	parts := strings.Split(s, ",")
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out = append(out, aws.String(p))
	}
	return out
}

func stringToTags(s string) (out []*elbv2.Tag) {
	rawTags := stringToAwsSlice(s)
	for _, rawTag := range rawTags {
		parts := strings.Split(*rawTag, "=")
		switch {
		case *rawTag == "":
			continue
		case len(parts) < 2:
			glog.Infof("Unable to parse `%s` into Key=Value pair", *rawTag)
			continue
		}
		out = append(out, &elbv2.Tag{
			Key:   aws.String(parts[0]),
			Value: aws.String(parts[1]),
		})
	}

	return out
}

func parseSubnets(s string) (out util.Subnets, err error) {
	var names []*string

	for _, subnet := range stringToAwsSlice(s) {
		if strings.HasPrefix(*subnet, "subnet-") {
			out = append(out, subnet)
			continue
		}

		item := cache.Get(*subnet)
		if item != nil {
			awsutil.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "hit"}).Add(float64(1))
			out = append(out, item.Value().(*string))
			continue
		}
		awsutil.AWSCache.With(prometheus.Labels{"cache": "subnets", "action": "miss"}).Add(float64(1))

		names = append(names, subnet)
	}

	if len(names) > 0 {
		in := ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{{
			Name:   aws.String("tag:Name"),
			Values: names,
		}}}

		subnets, err := awsutil.Ec2svc.DescribeSubnets(in)
		if err != nil {
			log.Errorf("Unable to fetch subnets %v: %v", "controller", in.Filters, err)
			return nil, err
		}

		for _, subnet := range subnets {
			value, ok := util.EC2Tags(subnet.Tags).Get("Name")
			if ok {
				cache.Set(value, subnet.SubnetId, time.Minute*60)
				out = append(out, subnet.SubnetId)
			}
		}
	}

	sort.Sort(util.AWSStringSlice(out))
	if len(out) == 0 {
		return nil, fmt.Errorf("unable to resolve any subnets from: %s", s)
	}
	return out, nil
}

func parseSecurityGroups(s string) (out util.AWSStringSlice, err error) {
	var names []*string

	for _, sg := range stringToAwsSlice(s) {
		if strings.HasPrefix(*sg, "sg-") {
			out = append(out, sg)
			continue
		}

		item := cache.Get(*sg)
		if item != nil {
			awsutil.AWSCache.With(prometheus.Labels{"cache": "securitygroups", "action": "hit"}).Add(float64(1))
			out = append(out, item.Value().(*string))
			continue
		}

		awsutil.AWSCache.With(prometheus.Labels{"cache": "securitygroups", "action": "miss"}).Add(float64(1))
		names = append(names, sg)
	}

	if len(names) > 0 {
		in := ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{{
			Name:   aws.String("tag:Name"),
			Values: names,
		}}}

		sgs, err := awsutil.Ec2svc.DescribeSecurityGroups(in)
		if err != nil {
			glog.Errorf("Unable to fetch security groups %v: %v", in.Filters, err)
			return nil, err
		}

		for _, sg := range sgs {
			value, ok := util.EC2Tags(sg.Tags).Get("Name")
			if ok {
				cache.Set(value, sg.GroupId, time.Minute*60)
				out = append(out, sg.GroupId)
			}
		}
	}

	sort.Sort(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("unable to resolve any security groups from: %s", s)
	}
	return out, nil
}
