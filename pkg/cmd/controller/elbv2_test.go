package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
)

type mockedELBV2ResponsesT struct {
	Error error
}

var (
	mockedELBV2responses *mockedELBV2ResponsesT
)

func setupELBV2() {
	elbv2svc = newELBV2(nil)
	elbv2svc.svc = &mockedELBV2Client{}
	mockedELBV2responses = &mockedELBV2ResponsesT{}
}

type mockedELBV2Client struct {
	elbv2iface.ELBV2API
}

func (m *mockedELBV2Client) CreateListener(input *elbv2.CreateListenerInput) (*elbv2.CreateListenerOutput, error) {
	output := &elbv2.CreateListenerOutput{
		Listeners: []*elbv2.Listener{
			&elbv2.Listener{
				Certificates:    input.Certificates,
				DefaultActions:  input.DefaultActions,
				ListenerArn:     aws.String("some:arn"),
				LoadBalancerArn: input.LoadBalancerArn,
				Port:            input.Port,
				Protocol:        input.Protocol,
				SslPolicy:       input.SslPolicy,
			},
		},
	}
	return output, mockedELBV2responses.Error
}

func (m *mockedELBV2Client) DeleteListener(input *elbv2.DeleteListenerInput) (*elbv2.DeleteListenerOutput, error) {
	output := &elbv2.DeleteListenerOutput{}
	return output, mockedELBV2responses.Error
}
