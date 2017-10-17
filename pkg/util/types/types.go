package types

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

const (
	IdleTimeoutKey = "idle_timeout.timeout_seconds"
)

type AWSStringSlice []*string
type Tags []*elbv2.Tag
type EC2Tags []*ec2.Tag

type AvailabilityZones []*elbv2.AvailabilityZone
type Subnets AWSStringSlice

func (n AWSStringSlice) Len() int           { return len(n) }
func (n AWSStringSlice) Less(i, j int) bool { return *n[i] < *n[j] }
func (n AWSStringSlice) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

func (n Tags) Len() int           { return len(n) }
func (n Tags) Less(i, j int) bool { return *n[i].Key < *n[j].Key }
func (n Tags) Swap(i, j int) {
	n[i].Key, n[j].Key, n[i].Value, n[j].Value = n[j].Key, n[i].Key, n[j].Value, n[i].Value
}

var logger *log.Logger

func init() {
	logger = log.New("util")
}

func DeepEqual(x, y interface{}) bool {
	b := awsutil.DeepEqual(x, y)
	if b == false {
		logger.Debugf("DeepEqual(%v, %v) found inequality", log.Prettify(x), log.Prettify(y))
	}
	return b
}

// NewAWSStringSlice converts a string with comma separated strings into an AWSStringSlice.
func NewAWSStringSlice(s string) (out AWSStringSlice) {
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

// Hash returns a hash representing security group names
func (a AWSStringSlice) Hash() *string {
	sort.Sort(a)
	hasher := md5.New()
	for _, str := range a {
		hasher.Write([]byte(*str))
	}
	output := hex.EncodeToString(hasher.Sum(nil))
	return aws.String(output)
}

func (t Tags) Hash() *string {
	sort.Sort(t)
	hasher := md5.New()
	hasher.Write([]byte(awsutil.Prettify(t)))
	output := hex.EncodeToString(hasher.Sum(nil))
	return aws.String(output)
}

func (t *Tags) Get(s string) (string, bool) {
	for _, tag := range *t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}

func (t EC2Tags) Get(s string) (string, bool) {
	for _, tag := range t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}

func SortedMap(m map[string]string) Tags {
	var t Tags
	for k, v := range m {
		t = append(t, &elbv2.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	sort.Sort(t)
	return t
}

func (az AvailabilityZones) AsSubnets() AWSStringSlice {
	var out []*string
	for _, a := range az {
		out = append(out, a.SubnetId)
	}
	return out
}

func (subnets Subnets) AsAvailabilityZones() AvailabilityZones {
	var out []*elbv2.AvailabilityZone
	for _, s := range subnets {
		out = append(out, &elbv2.AvailabilityZone{SubnetId: s, ZoneName: aws.String("")})
	}
	return out
}

func (s Subnets) String() string {
	var out string
	for _, sub := range s {
		out += *sub
	}
	return out
}
