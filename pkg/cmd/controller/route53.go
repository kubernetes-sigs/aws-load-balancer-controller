package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

type Route53 struct {
	*route53.Route53
}

func newRoute53(awsconfig *aws.Config) *Route53 {
	r := Route53{
		route53.New(session.New(awsconfig)),
	}
	return &r
}
