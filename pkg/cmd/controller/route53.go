package controller

import (
	"log"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

type Route53 struct {
	*route53.Route53
}

func newRoute53(awsconfig *aws.Config) *Route53 {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		log.Printf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	r := Route53{
		//route53.New(session.New(awsconfig)),
		route53.New(session),
	}
	return &r
}
