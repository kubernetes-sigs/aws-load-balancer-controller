package services

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	"github.com/aws/aws-sdk-go/service/globalaccelerator/globalacceleratoriface"
)

type GlobalAccelerator interface {
	globalacceleratoriface.GlobalAcceleratorAPI
}

// NewGlobalAccelerator constructs new GlobalAccelerator implementation.
func NewGlobalAccelerator(session *session.Session) GlobalAccelerator {
	return &defaultGlobalAcceleratorAPI{
		// global accelerator always needs `us-west-2`-region
		GlobalAcceleratorAPI: globalaccelerator.New(session, aws.NewConfig().WithRegion("us-west-2")),
	}
}

// default implementation for Global Accelerator.
type defaultGlobalAcceleratorAPI struct {
	globalacceleratoriface.GlobalAcceleratorAPI
}
