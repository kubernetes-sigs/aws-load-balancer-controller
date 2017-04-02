package awsutil

import (
	"crypto/md5"
	"encoding/hex"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
)


type AWSStringSlice []*string
type Tags []*elbv2.Tag
type EC2Tags []*ec2.Tag

func (n AWSStringSlice) Len() int           { return len(n) }
func (n AWSStringSlice) Less(i, j int) bool { return *n[i] < *n[j] }
func (n AWSStringSlice) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

func (n Tags) Len() int           { return len(n) }
func (n Tags) Less(i, j int) bool { return *n[i].Key < *n[j].Key }
func (n Tags) Swap(i, j int) {
	n[i].Key, n[j].Key, n[i].Value, n[j].Value = n[j].Key, n[i].Key, n[j].Value, n[i].Value
}

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
