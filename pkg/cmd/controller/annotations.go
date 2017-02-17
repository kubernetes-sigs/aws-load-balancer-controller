package controller

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
)

const (
	securityGroupsKey = "ingress.ticketmaster.com/security-groups"
	subnetsKey        = "ingress.ticketmaster.com/subnets"
	schemeKey         = "ingress.ticketmaster.com/scheme"
	tagsKey           = "ingress.ticketmaster.com/tags"
)

type annotationsT struct {
	subnets        []*string
	scheme         *string
	securityGroups []*string
	tags           []*elbv2.Tag
}

func parseAnnotations(annotations map[string]string) (*annotationsT, error) {
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

	resp = &annotationsT{
		subnets:        stringToAwsSlice(annotations[subnetsKey]),
		scheme:         aws.String(annotations[schemeKey]),
		securityGroups: stringToAwsSlice(annotations[securityGroupsKey]),
		tags:           stringToTags(annotations[tagsKey]),
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
