package types

import (
	"github.com/aws/aws-sdk-go/service/ec2"
)

type EC2Tags []*ec2.Tag

func (t EC2Tags) Get(s string) (string, bool) {
	for _, tag := range t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}
