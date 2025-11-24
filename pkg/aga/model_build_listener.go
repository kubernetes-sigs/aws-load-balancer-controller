package aga

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

// listenerBuilder builds Listener model resources
type listenerBuilder interface {
	Build(ctx context.Context, stack core.Stack, accelerator *agamodel.Accelerator, listeners []agaapi.GlobalAcceleratorListener) ([]*agamodel.Listener, error)
}

// NewListenerBuilder constructs new listenerBuilder
func NewListenerBuilder() listenerBuilder {
	return &defaultListenerBuilder{}
}

var _ listenerBuilder = &defaultListenerBuilder{}

type defaultListenerBuilder struct{}

// Build builds Listener model resources
func (b *defaultListenerBuilder) Build(ctx context.Context, stack core.Stack, accelerator *agamodel.Accelerator, listeners []agaapi.GlobalAcceleratorListener) ([]*agamodel.Listener, error) {
	if listeners == nil || len(listeners) == 0 {
		return nil, nil
	}

	var result []*agamodel.Listener
	for i, listener := range listeners {
		listenerModel, err := buildListener(ctx, stack, accelerator, listener, i)
		if err != nil {
			return nil, err
		}
		result = append(result, listenerModel)
	}
	return result, nil
}

// buildListener builds a single Listener model resource
func buildListener(ctx context.Context, stack core.Stack, accelerator *agamodel.Accelerator, listener agaapi.GlobalAcceleratorListener, index int) (*agamodel.Listener, error) {
	spec, err := buildListenerSpec(ctx, accelerator, listener)
	if err != nil {
		return nil, err
	}

	resourceID := fmt.Sprintf("Listener-%d", index)
	listenerModel := agamodel.NewListener(stack, resourceID, spec, accelerator)
	return listenerModel, nil
}

// buildListenerSpec builds the ListenerSpec for a single Listener model resource
func buildListenerSpec(ctx context.Context, accelerator *agamodel.Accelerator, listener agaapi.GlobalAcceleratorListener) (agamodel.ListenerSpec, error) {
	protocol, err := buildListenerProtocol(ctx, listener)
	if err != nil {
		return agamodel.ListenerSpec{}, err
	}

	portRanges, err := buildListenerPortRanges(ctx, listener)
	if err != nil {
		return agamodel.ListenerSpec{}, err
	}

	clientAffinity := buildListenerClientAffinity(ctx, listener)

	return agamodel.ListenerSpec{
		AcceleratorARN: accelerator.AcceleratorARN(),
		Protocol:       protocol,
		PortRanges:     portRanges,
		ClientAffinity: clientAffinity,
	}, nil
}

// buildListenerProtocol determines the protocol for the listener
func buildListenerProtocol(_ context.Context, listener agaapi.GlobalAcceleratorListener) (agamodel.Protocol, error) {
	if listener.Protocol == nil {
		// TODO: Auto-discovery feature - Auto-determine protocol from endpoints if nil
		// Return error until auto-discovery feature is implemented
		return "", errors.New("listener protocol must be specified (auto-discovery not yet implemented)")
	}

	switch *listener.Protocol {
	case agaapi.GlobalAcceleratorProtocolTCP:
		return agamodel.ProtocolTCP, nil
	case agaapi.GlobalAcceleratorProtocolUDP:
		return agamodel.ProtocolUDP, nil
	default:
		return "", errors.Errorf("unsupported protocol: %s", *listener.Protocol)
	}
}

// buildListenerPortRanges determines the port ranges for the listener
func buildListenerPortRanges(_ context.Context, listener agaapi.GlobalAcceleratorListener) ([]agamodel.PortRange, error) {
	if listener.PortRanges == nil {
		// TODO: Auto-discovery feature - Auto-determine port ranges from endpoints if nil
		// Return error until auto-discovery feature is implemented
		return []agamodel.PortRange{}, errors.New("listener port ranges must be specified (auto-discovery not yet implemented)")
	}

	var portRanges []agamodel.PortRange
	for _, pr := range *listener.PortRanges {
		// Required validations are already done webhooks and CEL
		portRanges = append(portRanges, agamodel.PortRange{
			FromPort: pr.FromPort,
			ToPort:   pr.ToPort,
		})
	}
	return portRanges, nil
}

// buildListenerClientAffinity determines the client affinity for the listener
func buildListenerClientAffinity(_ context.Context, listener agaapi.GlobalAcceleratorListener) agamodel.ClientAffinity {
	switch listener.ClientAffinity {
	case agaapi.ClientAffinitySourceIP:
		return agamodel.ClientAffinitySourceIP
	default:
		// Default to NONE as per AWS Global Accelerator behavior
		return agamodel.ClientAffinityNone
	}
}
