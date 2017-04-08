package config

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	awstool "github.com/aws/aws-sdk-go/aws/awsutil"
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
	portKey            = "alb.ingress.kubernetes.io/port"
	schemeKey          = "alb.ingress.kubernetes.io/scheme"
	securityGroupsKey  = "alb.ingress.kubernetes.io/security-groups"
	subnetsKey         = "alb.ingress.kubernetes.io/subnets"
	successCodesKey    = "alb.ingress.kubernetes.io/successCodes"
	tagsKey            = "alb.ingress.kubernetes.io/Tags"
)

type AnnotationsT struct {
	BackendProtocol *string
	CertificateArn  *string
	HealthcheckPath *string
	Port            []*int64
	Scheme          *string
	SecurityGroups  util.AWSStringSlice
	Subnets         util.Subnets
	SuccessCodes    *string
	Tags            []*elbv2.Tag
}

func ParseAnnotations(annotations map[string]string) (*AnnotationsT, error) {
	resp := &AnnotationsT{}

	sortedAnnotations := util.SortedMap(annotations)
	cacheKey := "annotations " + awstool.Prettify(sortedAnnotations)

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

	resp = &AnnotationsT{
		BackendProtocol: aws.String(annotations[backendProtocolKey]),
		Port:            parsePort(annotations[portKey], annotations[certificateArnKey]),
		Subnets:         subnets,
		Scheme:          scheme,
		SecurityGroups:  securitygroups,
		SuccessCodes:    aws.String(annotations[successCodesKey]),
		Tags:            stringToTags(annotations[tagsKey]),
		HealthcheckPath: parseHealthcheckPath(annotations[healthcheckPathKey]),
	}

	if cert, ok := annotations[certificateArnKey]; ok {
		resp.CertificateArn = aws.String(cert)
	}

	return resp, nil
}

func parsePort(port, certArn string) []*int64 {
	ports := []*int64{}

	switch {
	case port == "" && certArn == "":
		return append(ports, aws.Int64(int64(80)))
	case port == "" && certArn != "":
		return append(ports, aws.Int64(int64(443)))
	}

	for _, port := range strings.Split(port, ",") {
		p, _ := strconv.ParseInt(port, 10, 64)
		ports = append(ports, aws.Int64(p))
	}
	return ports
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

	// Verify Subnets resolved from annotation exist.
	if len(out) > 0 {
		descRequest := &ec2.DescribeSubnetsInput{
			SubnetIds: out,
		}
		_, err := awsutil.Ec2svc.Svc.DescribeSubnets(descRequest)
		if err != nil {
			log.Errorf("Subnets specified were invalid. subnets: %s | Error: %s.", "controller",
				s, err.Error())
			return nil, err
		}
	}

	if len(names) > 0 {
		descRequest := &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{{
			Name:   aws.String("tag:Name"),
			Values: names,
		}}}

		subnetInfo, err := awsutil.Ec2svc.Svc.DescribeSubnets(descRequest)
		if err != nil {
			log.Errorf("Unable to fetch subnets %v: %v", "controller", descRequest.Filters, err)
			return nil, err
		}

		for _, subnet := range subnetInfo.Subnets {
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
		descRequest := &ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{{
			Name:   aws.String("tag:Name"),
			Values: names,
		}}}

		securitygroupInfo, err := awsutil.Ec2svc.Svc.DescribeSecurityGroups(descRequest)
		if err != nil {
			glog.Errorf("Unable to fetch security groups %v: %v", descRequest.Filters, err)
			return nil, err
		}

		for _, sg := range securitygroupInfo.SecurityGroups {
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
