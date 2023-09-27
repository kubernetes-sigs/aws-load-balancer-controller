package globalaccelerator

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go/aws"
	gasdk "github.com/aws/aws-sdk-go/service/globalaccelerator"

	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

type EndpointManager interface {
	// AddEndpoint add an endpoint to a globalaccelerator endpoint group
	AddEndpoint(ctx context.Context, endpointGroupARN string, lbArn string) error

	// GetEndpoint gets the existing endpoint for an endpointgroup and load balancer
	GetEndpoint(ctx context.Context, endpointGroupARN string, lbArn string) (string, error)

	// DeleteEndpoints deletes an existing endpoint for an endpointgroup and load balancer
	DeleteEndpoint(ctx context.Context, endpointGroupARN string, lbArn string) error
}

func NewDefaultEndpointManager(gaClient services.GlobalAccelerator, logger logr.Logger) *defaultEndpointManager {
	return &defaultEndpointManager{
		gaClient: gaClient,
	}
}

var _ EndpointManager = &defaultEndpointManager{}

type defaultEndpointManager struct {
	gaClient services.GlobalAccelerator
}

func (m *defaultEndpointManager) AddEndpoint(ctx context.Context, endpointGroupARN string, lbArn string) error {
	_, err := m.gaClient.AddEndpoints(&gasdk.AddEndpointsInput{
		EndpointGroupArn: &endpointGroupARN,
		EndpointConfigurations: []*gasdk.EndpointConfiguration{
			{
				EndpointId: awssdk.String(lbArn),
			},
		},
	})

	if err != nil {
		return err
	}

	return nil
}

func (m *defaultEndpointManager) GetEndpoint(ctx context.Context, endpointGroupARN string, lbArn string) (string, error) {
	epResponse, err := m.gaClient.DescribeEndpointGroup(&gasdk.DescribeEndpointGroupInput{
		EndpointGroupArn: &endpointGroupARN,
	})

	if err != nil {
		return "", err
	}

	for _, endpoint := range epResponse.EndpointGroup.EndpointDescriptions {
		if *endpoint.EndpointId == lbArn {
			return lbArn, nil
		}
	}

	return "", nil
}

func (m *defaultEndpointManager) DeleteEndpoint(ctx context.Context, endpointGroupARN string, lbArn string) error {
	_, err := m.gaClient.RemoveEndpoints(&gasdk.RemoveEndpointsInput{
		EndpointGroupArn: &endpointGroupARN,
		EndpointIdentifiers: []*gasdk.EndpointIdentifier{
			{
				EndpointId: awssdk.String(lbArn),
			},
		},
	})
	if err != nil {
		return err
	}

	return nil
}
