package services

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
)

type GlobalAccelerator interface {
	// CreateAccelerator creates a new Global Accelerator accelerator.
	CreateAccelerator(ctx context.Context, input *globalaccelerator.CreateAcceleratorInput) (*globalaccelerator.CreateAcceleratorOutput, error)

	// DescribeAccelerator describes an existing Global Accelerator accelerator.
	DescribeAccelerator(ctx context.Context, input *globalaccelerator.DescribeAcceleratorInput) (*globalaccelerator.DescribeAcceleratorOutput, error)

	// UpdateAccelerator updates a Global Accelerator accelerator.
	UpdateAccelerator(ctx context.Context, input *globalaccelerator.UpdateAcceleratorInput) (*globalaccelerator.UpdateAcceleratorOutput, error)

	// DeleteAccelerator deletes a Global Accelerator accelerator.
	DeleteAccelerator(ctx context.Context, input *globalaccelerator.DeleteAcceleratorInput) (*globalaccelerator.DeleteAcceleratorOutput, error)

	// ListAccelerators lists Global Accelerator accelerators.
	ListAccelerators(ctx context.Context, input *globalaccelerator.ListAcceleratorsInput) (*globalaccelerator.ListAcceleratorsOutput, error)

	// CreateListener creates a new listener for a Global Accelerator accelerator.
	CreateListener(ctx context.Context, input *globalaccelerator.CreateListenerInput) (*globalaccelerator.CreateListenerOutput, error)

	// DescribeListener describes a Global Accelerator listener.
	DescribeListener(ctx context.Context, input *globalaccelerator.DescribeListenerInput) (*globalaccelerator.DescribeListenerOutput, error)

	// UpdateListener updates a Global Accelerator listener.
	UpdateListener(ctx context.Context, input *globalaccelerator.UpdateListenerInput) (*globalaccelerator.UpdateListenerOutput, error)

	// DeleteListener deletes a Global Accelerator listener.
	DeleteListener(ctx context.Context, input *globalaccelerator.DeleteListenerInput) (*globalaccelerator.DeleteListenerOutput, error)

	// ListListeners lists listeners for a Global Accelerator accelerator.
	ListListeners(ctx context.Context, input *globalaccelerator.ListListenersInput) (*globalaccelerator.ListListenersOutput, error)

	// CreateEndpointGroup creates a new endpoint group for a Global Accelerator listener.
	CreateEndpointGroup(ctx context.Context, input *globalaccelerator.CreateEndpointGroupInput) (*globalaccelerator.CreateEndpointGroupOutput, error)

	// DescribeEndpointGroup describes a Global Accelerator endpoint group.
	DescribeEndpointGroup(ctx context.Context, input *globalaccelerator.DescribeEndpointGroupInput) (*globalaccelerator.DescribeEndpointGroupOutput, error)

	// UpdateEndpointGroup updates a Global Accelerator endpoint group.
	UpdateEndpointGroup(ctx context.Context, input *globalaccelerator.UpdateEndpointGroupInput) (*globalaccelerator.UpdateEndpointGroupOutput, error)

	// DeleteEndpointGroup deletes a Global Accelerator endpoint group.
	DeleteEndpointGroup(ctx context.Context, input *globalaccelerator.DeleteEndpointGroupInput) (*globalaccelerator.DeleteEndpointGroupOutput, error)

	// ListEndpointGroups lists endpoint groups for a Global Accelerator listener.
	ListEndpointGroups(ctx context.Context, input *globalaccelerator.ListEndpointGroupsInput) (*globalaccelerator.ListEndpointGroupsOutput, error)

	// TagResource adds tags to a Global Accelerator resource.
	TagResource(ctx context.Context, input *globalaccelerator.TagResourceInput) (*globalaccelerator.TagResourceOutput, error)

	// UntagResource removes tags from a Global Accelerator resource.
	UntagResource(ctx context.Context, input *globalaccelerator.UntagResourceInput) (*globalaccelerator.UntagResourceOutput, error)

	// ListTagsForResource lists tags for a Global Accelerator resource.
	ListTagsForResource(ctx context.Context, input *globalaccelerator.ListTagsForResourceInput) (*globalaccelerator.ListTagsForResourceOutput, error)

	// UpdateAcceleratorAttributes updates the attributes of a Global Accelerator accelerator.
	UpdateAcceleratorAttributes(ctx context.Context, input *globalaccelerator.UpdateAcceleratorAttributesInput) (*globalaccelerator.UpdateAcceleratorAttributesOutput, error)

	// DescribeAcceleratorAttributes describes the attributes of a Global Accelerator accelerator.
	DescribeAcceleratorAttributes(ctx context.Context, input *globalaccelerator.DescribeAcceleratorAttributesInput) (*globalaccelerator.DescribeAcceleratorAttributesOutput, error)
}

// NewGlobalAccelerator constructs new GlobalAccelerator implementation.
func NewGlobalAccelerator(awsClientsProvider provider.AWSClientsProvider) GlobalAccelerator {
	globalacceleratorClient, _ := awsClientsProvider.GetGlobalAcceleratorClient(context.TODO(), "")
	return &defaultGlobalAccelerator{
		globalacceleratorClient: globalacceleratorClient,
	}
}

// globalAcceleratorClient is the interface for the Global Accelerator client used by this package.
type globalAcceleratorClient interface {
	CreateAccelerator(ctx context.Context, params *globalaccelerator.CreateAcceleratorInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.CreateAcceleratorOutput, error)
	DescribeAccelerator(ctx context.Context, params *globalaccelerator.DescribeAcceleratorInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.DescribeAcceleratorOutput, error)
	UpdateAccelerator(ctx context.Context, params *globalaccelerator.UpdateAcceleratorInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.UpdateAcceleratorOutput, error)
	DeleteAccelerator(ctx context.Context, params *globalaccelerator.DeleteAcceleratorInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.DeleteAcceleratorOutput, error)
	ListAccelerators(ctx context.Context, params *globalaccelerator.ListAcceleratorsInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.ListAcceleratorsOutput, error)
	CreateListener(ctx context.Context, params *globalaccelerator.CreateListenerInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.CreateListenerOutput, error)
	DescribeListener(ctx context.Context, params *globalaccelerator.DescribeListenerInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.DescribeListenerOutput, error)
	UpdateListener(ctx context.Context, params *globalaccelerator.UpdateListenerInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.UpdateListenerOutput, error)
	DeleteListener(ctx context.Context, params *globalaccelerator.DeleteListenerInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.DeleteListenerOutput, error)
	ListListeners(ctx context.Context, params *globalaccelerator.ListListenersInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.ListListenersOutput, error)
	CreateEndpointGroup(ctx context.Context, params *globalaccelerator.CreateEndpointGroupInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.CreateEndpointGroupOutput, error)
	DescribeEndpointGroup(ctx context.Context, params *globalaccelerator.DescribeEndpointGroupInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.DescribeEndpointGroupOutput, error)
	UpdateEndpointGroup(ctx context.Context, params *globalaccelerator.UpdateEndpointGroupInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.UpdateEndpointGroupOutput, error)
	DeleteEndpointGroup(ctx context.Context, params *globalaccelerator.DeleteEndpointGroupInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.DeleteEndpointGroupOutput, error)
	ListEndpointGroups(ctx context.Context, params *globalaccelerator.ListEndpointGroupsInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.ListEndpointGroupsOutput, error)
	TagResource(ctx context.Context, params *globalaccelerator.TagResourceInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.TagResourceOutput, error)
	UntagResource(ctx context.Context, params *globalaccelerator.UntagResourceInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.UntagResourceOutput, error)
	ListTagsForResource(ctx context.Context, params *globalaccelerator.ListTagsForResourceInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.ListTagsForResourceOutput, error)
	UpdateAcceleratorAttributes(ctx context.Context, params *globalaccelerator.UpdateAcceleratorAttributesInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.UpdateAcceleratorAttributesOutput, error)
	DescribeAcceleratorAttributes(ctx context.Context, params *globalaccelerator.DescribeAcceleratorAttributesInput, optFns ...func(*globalaccelerator.Options)) (*globalaccelerator.DescribeAcceleratorAttributesOutput, error)
}

type defaultGlobalAccelerator struct {
	globalacceleratorClient globalAcceleratorClient
}

func (c *defaultGlobalAccelerator) CreateAccelerator(ctx context.Context, input *globalaccelerator.CreateAcceleratorInput) (*globalaccelerator.CreateAcceleratorOutput, error) {
	return c.globalacceleratorClient.CreateAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) DescribeAccelerator(ctx context.Context, input *globalaccelerator.DescribeAcceleratorInput) (*globalaccelerator.DescribeAcceleratorOutput, error) {
	return c.globalacceleratorClient.DescribeAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) UpdateAccelerator(ctx context.Context, input *globalaccelerator.UpdateAcceleratorInput) (*globalaccelerator.UpdateAcceleratorOutput, error) {
	return c.globalacceleratorClient.UpdateAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) DeleteAccelerator(ctx context.Context, input *globalaccelerator.DeleteAcceleratorInput) (*globalaccelerator.DeleteAcceleratorOutput, error) {
	return c.globalacceleratorClient.DeleteAccelerator(ctx, input)
}

func (c *defaultGlobalAccelerator) ListAccelerators(ctx context.Context, input *globalaccelerator.ListAcceleratorsInput) (*globalaccelerator.ListAcceleratorsOutput, error) {
	return c.globalacceleratorClient.ListAccelerators(ctx, input)
}

func (c *defaultGlobalAccelerator) CreateListener(ctx context.Context, input *globalaccelerator.CreateListenerInput) (*globalaccelerator.CreateListenerOutput, error) {
	return c.globalacceleratorClient.CreateListener(ctx, input)
}

func (c *defaultGlobalAccelerator) DescribeListener(ctx context.Context, input *globalaccelerator.DescribeListenerInput) (*globalaccelerator.DescribeListenerOutput, error) {
	return c.globalacceleratorClient.DescribeListener(ctx, input)
}

func (c *defaultGlobalAccelerator) UpdateListener(ctx context.Context, input *globalaccelerator.UpdateListenerInput) (*globalaccelerator.UpdateListenerOutput, error) {
	return c.globalacceleratorClient.UpdateListener(ctx, input)
}

func (c *defaultGlobalAccelerator) DeleteListener(ctx context.Context, input *globalaccelerator.DeleteListenerInput) (*globalaccelerator.DeleteListenerOutput, error) {
	return c.globalacceleratorClient.DeleteListener(ctx, input)
}

func (c *defaultGlobalAccelerator) ListListeners(ctx context.Context, input *globalaccelerator.ListListenersInput) (*globalaccelerator.ListListenersOutput, error) {
	return c.globalacceleratorClient.ListListeners(ctx, input)
}

func (c *defaultGlobalAccelerator) CreateEndpointGroup(ctx context.Context, input *globalaccelerator.CreateEndpointGroupInput) (*globalaccelerator.CreateEndpointGroupOutput, error) {
	return c.globalacceleratorClient.CreateEndpointGroup(ctx, input)
}

func (c *defaultGlobalAccelerator) DescribeEndpointGroup(ctx context.Context, input *globalaccelerator.DescribeEndpointGroupInput) (*globalaccelerator.DescribeEndpointGroupOutput, error) {
	return c.globalacceleratorClient.DescribeEndpointGroup(ctx, input)
}

func (c *defaultGlobalAccelerator) UpdateEndpointGroup(ctx context.Context, input *globalaccelerator.UpdateEndpointGroupInput) (*globalaccelerator.UpdateEndpointGroupOutput, error) {
	return c.globalacceleratorClient.UpdateEndpointGroup(ctx, input)
}

func (c *defaultGlobalAccelerator) DeleteEndpointGroup(ctx context.Context, input *globalaccelerator.DeleteEndpointGroupInput) (*globalaccelerator.DeleteEndpointGroupOutput, error) {
	return c.globalacceleratorClient.DeleteEndpointGroup(ctx, input)
}

func (c *defaultGlobalAccelerator) ListEndpointGroups(ctx context.Context, input *globalaccelerator.ListEndpointGroupsInput) (*globalaccelerator.ListEndpointGroupsOutput, error) {
	return c.globalacceleratorClient.ListEndpointGroups(ctx, input)
}

func (c *defaultGlobalAccelerator) TagResource(ctx context.Context, input *globalaccelerator.TagResourceInput) (*globalaccelerator.TagResourceOutput, error) {
	return c.globalacceleratorClient.TagResource(ctx, input)
}

func (c *defaultGlobalAccelerator) UntagResource(ctx context.Context, input *globalaccelerator.UntagResourceInput) (*globalaccelerator.UntagResourceOutput, error) {
	return c.globalacceleratorClient.UntagResource(ctx, input)
}

func (c *defaultGlobalAccelerator) ListTagsForResource(ctx context.Context, input *globalaccelerator.ListTagsForResourceInput) (*globalaccelerator.ListTagsForResourceOutput, error) {
	return c.globalacceleratorClient.ListTagsForResource(ctx, input)
}

func (c *defaultGlobalAccelerator) UpdateAcceleratorAttributes(ctx context.Context, input *globalaccelerator.UpdateAcceleratorAttributesInput) (*globalaccelerator.UpdateAcceleratorAttributesOutput, error) {
	return c.globalacceleratorClient.UpdateAcceleratorAttributes(ctx, input)
}

func (c *defaultGlobalAccelerator) DescribeAcceleratorAttributes(ctx context.Context, input *globalaccelerator.DescribeAcceleratorAttributesInput) (*globalaccelerator.DescribeAcceleratorAttributesOutput, error) {
	return c.globalacceleratorClient.DescribeAcceleratorAttributes(ctx, input)
}
