package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type GlobalAccelerator interface {
	// wrapper to ListAcceleratorsPagesWithContext API, which aggregates paged results into list.
	ListAcceleratorsAsList(ctx context.Context, input *globalaccelerator.ListAcceleratorsInput) ([]types.Accelerator, error)

	// CreateAccelerator creates a new accelerator.
	CreateAcceleratorWithContext(ctx context.Context, input *globalaccelerator.CreateAcceleratorInput) (*globalaccelerator.CreateAcceleratorOutput, error)

	// DescribeAccelerator describes an accelerator.
	DescribeAcceleratorWithContext(ctx context.Context, input *globalaccelerator.DescribeAcceleratorInput) (*globalaccelerator.DescribeAcceleratorOutput, error)

	// UpdateAccelerator updates an accelerator.
	UpdateAcceleratorWithContext(ctx context.Context, input *globalaccelerator.UpdateAcceleratorInput) (*globalaccelerator.UpdateAcceleratorOutput, error)

	// DeleteAccelerator deletes an accelerator.
	DeleteAcceleratorWithContext(ctx context.Context, input *globalaccelerator.DeleteAcceleratorInput) (*globalaccelerator.DeleteAcceleratorOutput, error)

	// CreateListener creates a new listener.
	CreateListenerWithContext(ctx context.Context, input *globalaccelerator.CreateListenerInput) (*globalaccelerator.CreateListenerOutput, error)

	// DescribeListener describes a listener.
	DescribeListenerWithContext(ctx context.Context, input *globalaccelerator.DescribeListenerInput) (*globalaccelerator.DescribeListenerOutput, error)

	// UpdateListener updates a listener.
	UpdateListenerWithContext(ctx context.Context, input *globalaccelerator.UpdateListenerInput) (*globalaccelerator.UpdateListenerOutput, error)

	// DeleteListener deletes a listener.
	DeleteListenerWithContext(ctx context.Context, input *globalaccelerator.DeleteListenerInput) (*globalaccelerator.DeleteListenerOutput, error)

	// wrapper to ListListeners API, which aggregates paged results into list.
	ListListenersAsList(ctx context.Context, input *globalaccelerator.ListListenersInput) ([]types.Listener, error)

	// ListListenersForAccelerator lists all listeners for an accelerator.
	ListListenersForAcceleratorWithContext(ctx context.Context, input *globalaccelerator.ListListenersInput) (*globalaccelerator.ListListenersOutput, error)

	// TagResource tags a resource.
	TagResourceWithContext(ctx context.Context, input *globalaccelerator.TagResourceInput) (*globalaccelerator.TagResourceOutput, error)

	// UntagResource untags a resource.
	UntagResourceWithContext(ctx context.Context, input *globalaccelerator.UntagResourceInput) (*globalaccelerator.UntagResourceOutput, error)

	// ListTagsForResource lists tags for a resource.
	ListTagsForResourceWithContext(ctx context.Context, input *globalaccelerator.ListTagsForResourceInput) (*globalaccelerator.ListTagsForResourceOutput, error)
}

// NewGlobalAccelerator constructs new GlobalAccelerator implementation.
func NewGlobalAccelerator(awsClientsProvider provider.AWSClientsProvider) GlobalAccelerator {
	return &defaultGlobalAccelerator{
		awsClientsProvider: awsClientsProvider,
	}
}

// default implementation for GlobalAccelerator.
type defaultGlobalAccelerator struct {
	awsClientsProvider provider.AWSClientsProvider
}

func (c *defaultGlobalAccelerator) CreateAcceleratorWithContext(ctx context.Context, input *globalaccelerator.CreateAcceleratorInput) (*globalaccelerator.CreateAcceleratorOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "CreateAccelerator")
	if err != nil {
		return nil, err
	}
	return client.CreateAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) DescribeAcceleratorWithContext(ctx context.Context, input *globalaccelerator.DescribeAcceleratorInput) (*globalaccelerator.DescribeAcceleratorOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "DescribeAccelerator")
	if err != nil {
		return nil, err
	}
	return client.DescribeAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) UpdateAcceleratorWithContext(ctx context.Context, input *globalaccelerator.UpdateAcceleratorInput) (*globalaccelerator.UpdateAcceleratorOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "UpdateAccelerator")
	if err != nil {
		return nil, err
	}
	return client.UpdateAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) DeleteAcceleratorWithContext(ctx context.Context, input *globalaccelerator.DeleteAcceleratorInput) (*globalaccelerator.DeleteAcceleratorOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "DeleteAccelerator")
	if err != nil {
		return nil, err
	}
	return client.DeleteAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) TagResourceWithContext(ctx context.Context, input *globalaccelerator.TagResourceInput) (*globalaccelerator.TagResourceOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "TagResource")
	if err != nil {
		return nil, err
	}
	return client.TagResource(ctx, input)
}

func (c *defaultGlobalAccelerator) UntagResourceWithContext(ctx context.Context, input *globalaccelerator.UntagResourceInput) (*globalaccelerator.UntagResourceOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "UntagResource")
	if err != nil {
		return nil, err
	}
	return client.UntagResource(ctx, input)
}

func (c *defaultGlobalAccelerator) ListAcceleratorsAsList(ctx context.Context, input *globalaccelerator.ListAcceleratorsInput) ([]types.Accelerator, error) {
	var result []types.Accelerator
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "ListAccelerators")
	if err != nil {
		return nil, err
	}
	paginator := globalaccelerator.NewListAcceleratorsPaginator(client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Accelerators...)
	}
	return result, nil
}

func (c *defaultGlobalAccelerator) ListTagsForResourceWithContext(ctx context.Context, input *globalaccelerator.ListTagsForResourceInput) (*globalaccelerator.ListTagsForResourceOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "ListTagsForResource")
	if err != nil {
		return nil, err
	}
	return client.ListTagsForResource(ctx, input)
}

func (c *defaultGlobalAccelerator) CreateListenerWithContext(ctx context.Context, input *globalaccelerator.CreateListenerInput) (*globalaccelerator.CreateListenerOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "CreateListener")
	if err != nil {
		return nil, err
	}
	return client.CreateListener(ctx, input)
}

func (c *defaultGlobalAccelerator) DescribeListenerWithContext(ctx context.Context, input *globalaccelerator.DescribeListenerInput) (*globalaccelerator.DescribeListenerOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "DescribeListener")
	if err != nil {
		return nil, err
	}
	return client.DescribeListener(ctx, input)
}

func (c *defaultGlobalAccelerator) UpdateListenerWithContext(ctx context.Context, input *globalaccelerator.UpdateListenerInput) (*globalaccelerator.UpdateListenerOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "UpdateListener")
	if err != nil {
		return nil, err
	}
	return client.UpdateListener(ctx, input)
}

func (c *defaultGlobalAccelerator) DeleteListenerWithContext(ctx context.Context, input *globalaccelerator.DeleteListenerInput) (*globalaccelerator.DeleteListenerOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "DeleteListener")
	if err != nil {
		return nil, err
	}
	return client.DeleteListener(ctx, input)
}

func (c *defaultGlobalAccelerator) ListListenersForAcceleratorWithContext(ctx context.Context, input *globalaccelerator.ListListenersInput) (*globalaccelerator.ListListenersOutput, error) {
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "ListListeners")
	if err != nil {
		return nil, err
	}
	return client.ListListeners(ctx, input)
}

func (c *defaultGlobalAccelerator) ListListenersAsList(ctx context.Context, input *globalaccelerator.ListListenersInput) ([]types.Listener, error) {
	var result []types.Listener
	client, err := c.awsClientsProvider.GetGlobalAcceleratorClient(ctx, "ListListeners")
	if err != nil {
		return nil, err
	}
	paginator := globalaccelerator.NewListListenersPaginator(client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Listeners...)
	}
	return result, nil
}
