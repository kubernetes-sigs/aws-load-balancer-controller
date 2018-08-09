package types

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ELBv2Tags []*elbv2.Tag

func (n ELBv2Tags) Len() int           { return len(n) }
func (n ELBv2Tags) Less(i, j int) bool { return *n[i].Key < *n[j].Key }
func (n ELBv2Tags) Swap(i, j int) {
	n[i].Key, n[j].Key, n[i].Value, n[j].Value = n[j].Key, n[i].Key, n[j].Value, n[i].Value
}

func (t ELBv2Tags) Hash() string {
	sort.Sort(t)
	hasher := md5.New()
	hasher.Write([]byte(awsutil.Prettify(t)))
	output := hex.EncodeToString(hasher.Sum(nil))
	return output
}

func (t *ELBv2Tags) Get(s string) (string, bool) {
	for _, tag := range *t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}

func (t ELBv2Tags) Copy() ELBv2Tags {
	var tags ELBv2Tags
	for i := range t {
		tags = append(tags, &elbv2.Tag{
			Key:   aws.String(*t[i].Key),
			Value: aws.String(*t[i].Value),
		})
	}
	return tags
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
	} else {
		return "", intstr.IntOrString{}, fmt.Errorf("kubernetes.io/service-name tag is missing")
	}

	if v, ok := t.Get("kubernetes.io/service-port"); ok {
		port = intstr.Parse(v)
	} else {
		return "", intstr.IntOrString{}, fmt.Errorf("kubernetes.io/service-port is missing")
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
