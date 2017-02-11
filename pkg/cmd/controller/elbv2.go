package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
)

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	*elbv2.ELBV2
}

func newELBV2(awsconfig *aws.Config) *ELBV2 {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	elbv2 := ELBV2{
		elbv2.New(session),
	}
	return &elbv2
}
