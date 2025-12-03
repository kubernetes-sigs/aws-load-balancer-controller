package aga

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listenerBuilder builds Listener model resources
type listenerBuilder interface {
	Build(ctx context.Context, stack core.Stack, accelerator *agamodel.Accelerator, listeners []agaapi.GlobalAcceleratorListener, ga *agaapi.GlobalAccelerator, loadedEndpoints []*LoadedEndpoint) ([]*agamodel.Listener, []agaapi.GlobalAcceleratorListener, error)
}

// NewListenerBuilder constructs new listenerBuilder
func NewListenerBuilder(k8sClient client.Client, logger logr.Logger, elbv2Client services.ELBV2) listenerBuilder {

	endpointDiscovery := NewEndpointDiscovery(k8sClient, logger, elbv2Client)

	return &defaultListenerBuilder{
		endpointDiscovery: endpointDiscovery,
		logger:            logger,
	}
}

var _ listenerBuilder = &defaultListenerBuilder{}

type defaultListenerBuilder struct {
	endpointDiscovery *EndpointDiscovery
	logger            logr.Logger
}

// Build builds Listener model resources
func (b *defaultListenerBuilder) Build(ctx context.Context, stack core.Stack, accelerator *agamodel.Accelerator, listeners []agaapi.GlobalAcceleratorListener, ga *agaapi.GlobalAccelerator, loadedEndpoints []*LoadedEndpoint) ([]*agamodel.Listener, []agaapi.GlobalAcceleratorListener, error) {
	if listeners == nil || len(listeners) == 0 {
		return nil, nil, nil
	}

	var listenersToProcess []agaapi.GlobalAcceleratorListener

	// Default to using original listeners
	listenersToProcess = listeners

	// Apply auto-discovery logic if applicable
	canApplyAutoDiscovery := canApplyAutoDiscoveryForGA(ga, loadedEndpoints)
	if canApplyAutoDiscovery {
		var err error
		listenersToProcess, err = b.buildAutoDiscoveryListeners(ctx, listeners[0], loadedEndpoints[0], ga)
		if err != nil {
			return nil, nil, err
		}
	}

	var result []*agamodel.Listener
	for i, listener := range listenersToProcess {
		listenerModel, err := buildListener(ctx, stack, accelerator, listener, i)
		if err != nil {
			return nil, nil, err
		}
		result = append(result, listenerModel)
	}
	return result, listenersToProcess, nil
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
		return "", errors.New("listener protocol must be specified ")
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
		return []agamodel.PortRange{}, errors.New("listener port ranges must be specified")
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

// buildAutoDiscoveryListeners creates listeners based on auto-discovered protocols and ports
// This function is responsible for:
// 1. Fetching protocols and ports information from the loaded endpoint
// 2. Determining which protocols to create listeners for, based on protocol specification in the input
// 3. Creating appropriate listeners for each protocol with their corresponding port ranges
// 4. Consolidating ports into ranges to optimize AWS resources
//
// Returns new listeners with auto-discovered protocols and ports
func (b *defaultListenerBuilder) buildAutoDiscoveryListeners(
	ctx context.Context,
	templateListener agaapi.GlobalAcceleratorListener,
	loadedEndpoint *LoadedEndpoint,
	ga *agaapi.GlobalAccelerator) ([]agaapi.GlobalAcceleratorListener, error) {

	// Pre-fetch the protocol information
	protocolPortsInfo, discoveryErr := b.endpointDiscovery.FetchProtocolPortInfo(ctx, loadedEndpoint)
	if discoveryErr != nil {
		b.logger.Error(discoveryErr, "failed to fetch endpoint port info for auto-discovery",
			"endpoint", loadedEndpoint.Name,
			"accelerator", k8s.NamespacedName(ga))
		return nil, errors.Wrap(discoveryErr, "failed to fetch endpoint port info for auto-discovery")
	}

	// Check if we have any protocol information to work with
	if len(protocolPortsInfo) == 0 {
		err := errors.New("no protocol or port information found for auto-discovery")
		b.logger.Error(err, "unable to auto-discover listener configuration",
			"endpoint", loadedEndpoint.Name,
			"accelerator", k8s.NamespacedName(ga))
		return nil, err
	}

	// Determine which protocols to create listeners for
	var protocolsToCreate []agaapi.GlobalAcceleratorProtocol
	if templateListener.Protocol != nil {
		// Explicitly specified protocol - create single listener
		protocolsToCreate = []agaapi.GlobalAcceleratorProtocol{*templateListener.Protocol}
	} else if len(protocolPortsInfo) > 1 {
		// Multiple protocols detected - will always be TCP and UDP only
		// Create one listener for each protocol
		protocolsToCreate = []agaapi.GlobalAcceleratorProtocol{
			agaapi.GlobalAcceleratorProtocolTCP,
			agaapi.GlobalAcceleratorProtocolUDP,
		}
	} else if len(protocolPortsInfo) == 1 {
		// Single protocol - create one listener
		protocolsToCreate = []agaapi.GlobalAcceleratorProtocol{protocolPortsInfo[0].Protocol}
	}

	// Create new listeners for each protocol detected
	listenersToProcess := make([]agaapi.GlobalAcceleratorListener, 0, len(protocolsToCreate))

	// Create listeners for each protocol
	for _, protocol := range protocolsToCreate {
		// Get matching ports for this protocol
		var matchingPorts []int32
		for _, info := range protocolPortsInfo {
			if info.Protocol == protocol {
				matchingPorts = append(matchingPorts, info.Ports...)
			}
		}

		// Consolidate ports into ranges
		var portRanges []agamodel.PortRange
		if len(matchingPorts) > 0 {
			portRanges = consolidatePortRanges(matchingPorts)
		}

		// Create new listener with protocol and port ranges
		newListener := createNewListener(templateListener, protocol, portRanges)
		listenersToProcess = append(listenersToProcess, newListener)

		b.logger.V(1).Info(
			"Created auto-discovery listener with port ranges",
			"protocol", protocol,
			"portCount", len(matchingPorts),
			"rangeCount", len(portRanges),
		)
	}

	return listenersToProcess, nil
}

// createNewListener creates a new listener with specified protocol and optional port ranges
func createNewListener(template agaapi.GlobalAcceleratorListener, protocol agaapi.GlobalAcceleratorProtocol, portRanges []agamodel.PortRange) agaapi.GlobalAcceleratorListener {
	var newListener agaapi.GlobalAcceleratorListener

	// Set protocol to the specified protocol
	newListener.Protocol = &protocol

	// Copy non-pointer fields directly
	newListener.ClientAffinity = template.ClientAffinity

	// Set port ranges - prioritize template port ranges if they exist
	if template.PortRanges != nil {
		portRangesCopy := make([]agaapi.PortRange, len(*template.PortRanges))
		copy(portRangesCopy, *template.PortRanges)
		newListener.PortRanges = &portRangesCopy
	} else if len(portRanges) > 0 {
		apiPortRanges := make([]agaapi.PortRange, 0, len(portRanges))
		for _, pr := range portRanges {
			apiPortRanges = append(apiPortRanges, agaapi.PortRange{
				FromPort: pr.FromPort,
				ToPort:   pr.ToPort,
			})
		}
		newListener.PortRanges = &apiPortRanges
	}

	// Copy EndpointGroups
	if template.EndpointGroups != nil {
		endpointGroupsCopy := make([]agaapi.GlobalAcceleratorEndpointGroup, len(*template.EndpointGroups))
		for i, eg := range *template.EndpointGroups {
			endpointGroupsCopy[i] = eg

			// Create new slice for Endpoints if they exist
			if eg.Endpoints != nil {
				endpointsCopy := make([]agaapi.GlobalAcceleratorEndpoint, len(*eg.Endpoints))
				copy(endpointsCopy, *eg.Endpoints)
				endpointGroupsCopy[i].Endpoints = &endpointsCopy
			}

			// Deep copy Region if it exists
			if eg.Region != nil {
				region := *eg.Region
				endpointGroupsCopy[i].Region = &region
			}

			// Deep copy TrafficDialPercentage if it exists
			if eg.TrafficDialPercentage != nil {
				trafficDialPct := *eg.TrafficDialPercentage
				endpointGroupsCopy[i].TrafficDialPercentage = &trafficDialPct
			}

			// Copy port overrides if they exist
			if eg.PortOverrides != nil {
				portOverridesCopy := make([]agaapi.PortOverride, len(*eg.PortOverrides))
				copy(portOverridesCopy, *eg.PortOverrides)
				endpointGroupsCopy[i].PortOverrides = &portOverridesCopy
			}
		}
		newListener.EndpointGroups = &endpointGroupsCopy
	}

	return newListener
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
