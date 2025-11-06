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
