package controller

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
)

const (
	healthcheckPathKey = "ingress.ticketmaster.com/healthcheck-path"
	schemeKey          = "ingress.ticketmaster.com/scheme"
	securityGroupsKey  = "ingress.ticketmaster.com/security-groups"
	subnetsKey         = "ingress.ticketmaster.com/subnets"
	tagsKey            = "ingress.ticketmaster.com/tags"
)

type annotationsT struct {
	healthcheckPath *string
	scheme          *string
	securityGroups  []*string
	subnets         []*string
	tags            []*elbv2.Tag
}

func (ac *ALBController) parseAnnotations(annotations map[string]string) (*annotationsT, error) {
	resp := &annotationsT{}

	// Verify required annotations present and are valid
	switch {
	case annotations[subnetsKey] == "":
		return resp, fmt.Errorf(`Necessary annotations missing. Must include %s`, subnetsKey)
	case annotations[schemeKey] == "":
		return resp, fmt.Errorf(`Necessary annotations missing. Must include %s`, schemeKey)
	case annotations[schemeKey] != "internal" && annotations[schemeKey] != "internet-facing":
		return resp, fmt.Errorf("ALB scheme [%v] must be either `internal` or `internet-facing`", annotations[schemeKey])
	}

	subnets := ac.parseSubnets(annotations[subnetsKey])
	securitygroups := ac.parseSecurityGroups(annotations[securityGroupsKey])

	resp = &annotationsT{
		subnets:         subnets,
		scheme:          aws.String(annotations[schemeKey]),
		securityGroups:  securitygroups,
		tags:            stringToTags(annotations[tagsKey]),
		healthcheckPath: aws.String(anotations[healthcheckPathKey]),
	}

	return resp, nil
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

	subnetInfo, err := ac.elbv2svc.EC2.DescribeSubnets(descRequest)
	if err != nil {
		glog.Errorf("Unable to fetch subnets %v: %v", descRequest.Filters, err)
		return out
	}

	for _, subnet := range subnetInfo.Subnets {
		out = append(out, subnet.SubnetId)
	}

	return out
}

func (ac *ALBController) parseSecurityGroups(s string) (out []*string) {
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

	securitygroupInfo, err := ac.elbv2svc.EC2.DescribeSecurityGroups(descRequest)
	if err != nil {
		glog.Errorf("Unable to fetch security groups %v: %v", descRequest.Filters, err)
		return out
	}

	for _, sg := range securitygroupInfo.SecurityGroups {
		out = append(out, sg.GroupId)
	}

	return out
}
