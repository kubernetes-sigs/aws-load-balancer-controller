package controller

import(
	"log"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/aws/session"
)

type ALB struct {
	*elbv2.ELBV2
}

func newALB(awsconfig *aws.Config) *ALB {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		log.Printf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	alb := ALB{
		elbv2.New(session),
	}
	return &alb
}