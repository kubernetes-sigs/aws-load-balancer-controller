package controller

import (
	"fmt"
	"strings"

	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
)

const (
	certificateArnKey  = "ingress.ticketmaster.com/certificate-arn"
	healthcheckPathKey = "ingress.ticketmaster.com/healthcheck-path"
	portKey            = "ingress.ticketmaster.com/port"
	schemeKey          = "ingress.ticketmaster.com/scheme"
	securityGroupsKey  = "ingress.ticketmaster.com/security-groups"
	subnetsKey         = "ingress.ticketmaster.com/subnets"
	successCodesKey    = "ingress.ticketmaster.com/successCodes"
	tagsKey            = "ingress.ticketmaster.com/tags"
)

type annotationsT struct {
	certificateArn  *string
	healthcheckPath *string
	port            *int64
	scheme          *string
	securityGroups  []*string
	subnets         []*string
	successCodes    *string
	tags            []*elbv2.Tag
}

func (ac *ALBController) parseAnnotations(annotations map[string]string) (*annotationsT, error) {
	resp := &annotationsT{}

	// Verify required annostations present and are valid
	switch {
	case annotations[successCodesKey] == "":
		annotations[successCodesKey] = "200"
	case annotations[subnetsKey] == "":
		return resp, fmt.Errorf(`Necessary annotations missing. Must include %s`, subnetsKey)
	}

	subnets := ac.parseSubnets(annotations[subnetsKey])
	securitygroups := parseSecurityGroups(annotations[securityGroupsKey])
	scheme, err := parseScheme(annotations[schemeKey])
	if err != nil {
		return nil, err
	}

	resp = &annotationsT{
		port:            parsePort(annotations[portKey], annotations[certificateArnKey]),
		subnets:         subnets,
		scheme:          scheme,
		securityGroups:  securitygroups,
		successCodes:    aws.String(annotations[successCodesKey]),
		tags:            stringToTags(annotations[tagsKey]),
		healthcheckPath: parseHealthcheckPath(annotations[healthcheckPathKey]),
	}

	// TODO create a helper func for this so we can get nils easier
	if cert, ok := annotations[certificateArnKey]; ok {
		resp.certificateArn = aws.String(cert)
	}

	return resp, nil
}

func parsePort(port, certArn string) *int64 {
	switch {
	case port == "" && certArn == "":
		return aws.Int64(int64(80))
	case port == "" && certArn != "":
		return aws.Int64(int64(443))
	}
	p, _ := strconv.ParseInt(port, 10, 64)
	return aws.Int64(p)
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
		return aws.String(""), fmt.Errorf("ALB scheme [%v] must be either `internal` or `internet-facing`", s)
	}
	return aws.String(s), nil
}

func stringToAwsSlice(s string) (out []*string) {
	parts := strings.Split(s, ",")
	for _, part := range parts {
		out = append(out, aws.String(strings.TrimSpace(part)))
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

func (ac *ALBController) parseSubnets(s string) (out []*string) {
	var names []*string

	for _, subnet := range stringToAwsSlice(s) {
		if strings.HasPrefix(*subnet, "subnet-") {
			out = append(out, subnet)
			continue
		}
		names = append(names, subnet)

	}

	descRequest := &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{&ec2.Filter{
		Name:   aws.String("tag:Name"),
		Values: names,
	}}}

	subnetInfo, err := ec2svc.svc.DescribeSubnets(descRequest)
	if err != nil {
		glog.Errorf("Unable to fetch subnets %v: %v", descRequest.Filters, err)
		return out
	}

	for _, subnet := range subnetInfo.Subnets {
		out = append(out, subnet.SubnetId)
	}

	return out
}

func parseSecurityGroups(s string) (out []*string) {
	var names []*string

	for _, sg := range stringToAwsSlice(s) {
		if strings.HasPrefix(*sg, "sg-") {
			out = append(out, sg)
			continue
		}
		names = append(names, sg)
	}

	descRequest := &ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{&ec2.Filter{
		Name:   aws.String("tag:Name"),
		Values: names,
	}}}

	securitygroupInfo, err := ec2svc.svc.DescribeSecurityGroups(descRequest)
	if err != nil {
		glog.Errorf("Unable to fetch security groups %v: %v", descRequest.Filters, err)
		return out
	}

	for _, sg := range securitygroupInfo.SecurityGroups {
		out = append(out, sg.GroupId)
	}

	return out
}
