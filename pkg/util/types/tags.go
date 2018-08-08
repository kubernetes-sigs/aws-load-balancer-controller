package types

import (
	"crypto/md5"
	"encoding/hex"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
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

type EC2Tags []*ec2.Tag

func (t EC2Tags) Get(s string) (string, bool) {
	for _, tag := range t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}
