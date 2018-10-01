package types

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ELBv2Tags []*elbv2.Tag

func (t *ELBv2Tags) Get(s string) (string, bool) {
	for _, tag := range *t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}

// ServiceNameAndPort returns the service-name and service-port tag values
func (t ELBv2Tags) ServiceNameAndPort() (name string, port intstr.IntOrString, err error) {
	// Support legacy tags
	if v, ok := t.Get("ServiceName"); ok {
		name = v
	}

	if v, ok := t.Get("kubernetes.io/service-name"); ok {
		p := strings.Split(v, "/")
		if len(p) < 2 {
			name = v
		} else {
			name = p[1]
		}
	}

	if name == "" {
		return "", intstr.IntOrString{}, fmt.Errorf("%v tag is missing", "kubernetes.io/service-name")
	}

	if v, ok := t.Get("kubernetes.io/service-port"); ok {
		port = intstr.Parse(v)
	} else {
		port = intstr.Parse("0")
	}

	return name, port, nil
}

type EC2Tags []*ec2.Tag

func (t EC2Tags) Get(s string) (string, bool) {
	for _, tag := range t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}
