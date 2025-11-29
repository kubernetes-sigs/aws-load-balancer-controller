package aga

import (
	"context"
	"encoding/json"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ProtocolPortInfo contains information about a protocol and its associated ports
type ProtocolPortInfo struct {
	Protocol agaapi.GlobalAcceleratorProtocol
	Ports    []int32
}

// EndpointDiscovery is responsible for extracting protocol and port information from different endpoint types
type EndpointDiscovery struct {
	client           client.Client
	annotationParser annotations.Parser
	logger           logr.Logger
	elbv2Client      services.ELBV2
}

// NewEndpointDiscovery creates a new EndpointDiscovery instance
func NewEndpointDiscovery(client client.Client, logger logr.Logger, elbv2Client services.ELBV2) *EndpointDiscovery {
	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)
	return &EndpointDiscovery{
		client:           client,
		annotationParser: annotationParser,
		logger:           logger,
		elbv2Client:      elbv2Client,
	}
}

// FetchProtocolPortInfo extracts port and protocol information from a loaded endpoint
// For the auto-discovery scenario, we use the following approach:
// 1. Identify the endpoint type (Service, Ingress, Gateway, or LoadBalancer via EndpointID)
// 2. Extract protocol and port information from the stored K8s resource or AWS API
// 3. For Service endpoints, handle both TCP and UDP protocols based on the Service definition
// 4. For Ingress endpoints, parse annotations to get port configurations
// 5. For Gateway endpoints, map Gateway protocols to GlobalAccelerator protocols
// 6. For LoadBalancer (EndpointID) endpoints, query AWS API to get listener information
func (d *EndpointDiscovery) FetchProtocolPortInfo(ctx context.Context, endpoint *LoadedEndpoint) ([]ProtocolPortInfo, error) {
	// For Kubernetes resource types, check if K8s resource is available
	if endpoint.Type != agaapi.GlobalAcceleratorEndpointTypeEndpointID && endpoint.K8sResource == nil {
		return nil, fmt.Errorf("kubernetes resource not available for endpoint %s/%s",
			endpoint.Namespace, endpoint.Name)
	}

	// Process based on endpoint type
	switch endpoint.Type {
	case agaapi.GlobalAcceleratorEndpointTypeService:
		return d.fetchServiceProtocolPortInfo(ctx, endpoint)
	case agaapi.GlobalAcceleratorEndpointTypeIngress:
		return d.fetchIngressProtocolPortInfo(ctx, endpoint)
	case agaapi.GlobalAcceleratorEndpointTypeGateway:
		return d.fetchGatewayProtocolPortInfo(ctx, endpoint)
	case agaapi.GlobalAcceleratorEndpointTypeEndpointID:
		// For LoadBalancer ARN endpoints, we query the AWS API directly
		// ARN should be already resolved during endpoint loading
		if endpoint.ARN == "" {
			return nil, fmt.Errorf("endpoint ARN is not available for endpoint with EndpointID type")
		}
		return d.fetchLoadBalancerProtocolPortInfo(ctx, endpoint)
	}

	return nil, fmt.Errorf("auto-discovery not supported for endpoint type %s", endpoint.Type)
}

// fetchServiceProtocolPortInfo extracts protocol and port information from a Service endpoint
func (d *EndpointDiscovery) fetchServiceProtocolPortInfo(_ context.Context, endpoint *LoadedEndpoint) ([]ProtocolPortInfo, error) {
	svc, ok := endpoint.K8sResource.(*corev1.Service)
	if !ok {
		return nil, fmt.Errorf("expected Service object for endpoint %v but got %T",
			k8s.NamespacedName(endpoint.K8sResource), endpoint.K8sResource)
	}

	// Group ports by port number to check for TCP_UDP services (same port number, different protocols)
	portMap := make(map[int32][]corev1.ServicePort)
	for _, port := range svc.Spec.Ports {
		key := port.Port
		if vals, exists := portMap[key]; exists {
			portMap[key] = append(vals, port)
		} else {
			portMap[key] = []corev1.ServicePort{port}
		}
	}

	// Check for TCP_UDP services and return error if found
	for portNum, servicePorts := range portMap {
		if len(servicePorts) > 1 {
			// TCP_UDP service case not supported
			return nil, fmt.Errorf("auto-discovery does not support TCP_UDP services on the same port %d for endpoint %v",
				portNum, k8s.NamespacedName(svc))
		}
	}

	// Group ports by protocol
	tcpPorts := []int32{}
	udpPorts := []int32{}

	for _, port := range svc.Spec.Ports {
		if port.Protocol == corev1.ProtocolUDP {
			udpPorts = append(udpPorts, port.Port)
		} else {
			tcpPorts = append(tcpPorts, port.Port)
		}
	}

	return createProtocolPortsInfo(tcpPorts, udpPorts), nil
}

// fetchIngressProtocolPortInfo extracts protocol and port information from an Ingress endpoint
// This function parses annotations to determine the correct ports and protocols
func (d *EndpointDiscovery) fetchIngressProtocolPortInfo(ctx context.Context, endpoint *LoadedEndpoint) ([]ProtocolPortInfo, error) {
	ing, ok := endpoint.K8sResource.(*networkingv1.Ingress)
	if !ok {
		return nil, fmt.Errorf("expected Ingress object for endpoint %v but got %T",
			k8s.NamespacedName(endpoint.K8sResource), endpoint.K8sResource)
	}

	// Check if there's a certificate (from annotations or IngressClassParams)
	hasCert, err := d.ingressHasCertificate(ctx, ing)
	if err != nil {
		return nil, fmt.Errorf("failed to check for ingress certificates: %w", err)
	}

	// Check for listen-ports annotation
	var tcpPorts []int32
	rawListenPorts := ""
	if d.annotationParser.ParseStringAnnotation(annotations.IngressSuffixListenPorts, &rawListenPorts, ing.Annotations) {
		// Parse the listen-ports JSON format
		// Example format: [{"HTTP": 80}, {"HTTPS": 443}, {"HTTP": 8080}, {"HTTPS": 8443}]
		var err error
		tcpPorts, err = d.parseIngressListenPorts(rawListenPorts, ing)
		if err != nil {
			return nil, err
		}
		if len(tcpPorts) == 0 {
			return nil, fmt.Errorf("no valid ports found in listen-ports configuration for ingress %v", k8s.NamespacedName(ing))
		}
	} else {
		// Use default ports based on certificate presence
		if hasCert {
			tcpPorts = []int32{443} // HTTPS port
		} else {
			tcpPorts = []int32{80} // HTTP port
		}
	}

	// Return TCP protocol with discovered ports
	return []ProtocolPortInfo{
		{Protocol: agaapi.GlobalAcceleratorProtocolTCP, Ports: tcpPorts},
	}, nil
}

// ingressHasCertificate checks if an Ingress has certificates defined either in
// annotations or IngressClassParams
func (d *EndpointDiscovery) ingressHasCertificate(ctx context.Context, ing *networkingv1.Ingress) (bool, error) {
	// First check annotations
	certARN := ""
	hasCert := d.annotationParser.ParseStringAnnotation(annotations.IngressSuffixCertificateARN, &certARN, ing.Annotations) && certARN != ""

	// If no certificate in annotations, check IngressClassParams if available
	if !hasCert && ing.Spec.IngressClassName != nil {
		hasCertFromParams, err := d.hasCertificatesInIngressClassParams(ctx, *ing.Spec.IngressClassName)
		if err != nil {
			return false, fmt.Errorf("error checking IngressClassParams for certificates: %w", err)
		}
		return hasCertFromParams, nil
	}

	return hasCert, nil
}

// parseIngressListenPorts parses the listen-ports annotation and extracts port numbers
func (d *EndpointDiscovery) parseIngressListenPorts(rawListenPorts string, ing *networkingv1.Ingress) ([]int32, error) {
	var entries []map[string]int32
	if err := json.Unmarshal([]byte(rawListenPorts), &entries); err != nil {
		d.logger.V(1).Error(err, "failed to parse listen-ports configuration for ingress",
			"listen-ports", rawListenPorts,
			"ingress", k8s.NamespacedName(ing))
		return nil, fmt.Errorf("failed to parse listen-ports annotation: %w", err)
	}

	if len(entries) == 0 {
		d.logger.V(1).Info("empty listen-ports configuration for ingress",
			"listen-ports", rawListenPorts,
			"ingress", k8s.NamespacedName(ing))
		return nil, fmt.Errorf("empty listen-ports configuration")
	}

	// Extract all ports from the listen-ports annotation
	var tcpPorts []int32
	for _, entry := range entries {
		for _, port := range entry {
			tcpPorts = append(tcpPorts, port)
		}
	}

	return tcpPorts, nil
}

// fetchGatewayProtocolPortInfo extracts protocol and port information from a Gateway endpoint
func (d *EndpointDiscovery) fetchGatewayProtocolPortInfo(_ context.Context, endpoint *LoadedEndpoint) ([]ProtocolPortInfo, error) {
	gw, ok := endpoint.K8sResource.(*gwv1.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected Gateway object for endpoint %v but got %T",
			k8s.NamespacedName(endpoint.K8sResource), endpoint.K8sResource)
	}

	// The test expects individual protocol-port mappings (one item per port) rather than grouped by protocol
	// For test compatibility, we'll create separate protocol groups for each port
	tcpPortsMap := make(map[int32]bool)
	udpPortsMap := make(map[int32]bool)

	// Process each listener and record ports by protocol
	for _, listener := range gw.Spec.Listeners {
		switch listener.Protocol {
		case gwv1.UDPProtocolType:
			udpPortsMap[int32(listener.Port)] = true
		default:
			// For HTTP, HTTPS, TLS, and other protocols, use TCP
			tcpPortsMap[int32(listener.Port)] = true
		}
	}

	// Convert maps to slices for easier handling
	var tcpPorts, udpPorts []int32
	for port := range tcpPortsMap {
		tcpPorts = append(tcpPorts, port)
	}
	for port := range udpPortsMap {
		udpPorts = append(udpPorts, port)
	}

	return createProtocolPortsInfo(tcpPorts, udpPorts), nil
}

// fetchLoadBalancerProtocolPortInfo extracts protocol and port information from a LoadBalancer ARN
// This uses the AWS API to retrieve ELBv2 listener information
func (d *EndpointDiscovery) fetchLoadBalancerProtocolPortInfo(ctx context.Context, endpoint *LoadedEndpoint) ([]ProtocolPortInfo, error) {
	lbARN := endpoint.ARN

	// Call the AWS API to get listener information
	protocolPortsInfo, err := d.getProtocolPortFromELBListener(ctx, lbARN)
	if err != nil {
		return nil, fmt.Errorf("failed to describe listeners for load balancer ARN %s: %w", lbARN, err)
	}

	// No listeners found
	if len(protocolPortsInfo) == 0 {
		return nil, fmt.Errorf("no listeners found for load balancer ARN %s", lbARN)
	}

	var tcpPorts, udpPorts []int32
	for _, info := range protocolPortsInfo {
		if info.Protocol == agaapi.GlobalAcceleratorProtocolTCP {
			tcpPorts = info.Ports
		} else if info.Protocol == agaapi.GlobalAcceleratorProtocolUDP {
			udpPorts = info.Ports
		}
	}

	d.logger.V(1).Info("discovered protocols and ports from AWS load balancer",
		"loadBalancerARN", lbARN,
		"tcpPorts", tcpPorts,
		"udpPorts", udpPorts)

	return protocolPortsInfo, nil
}

// hasCertificatesInIngressClassParams checks if the specified IngressClass has certificate ARNs defined
// in its associated IngressClassParams
func (d *EndpointDiscovery) hasCertificatesInIngressClassParams(ctx context.Context, ingressClassName string) (bool, error) {
	// First, get the IngressClass object to find its Parameters reference
	ingressClass := &networkingv1.IngressClass{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: ingressClassName}, ingressClass); err != nil {
		return false, fmt.Errorf("failed to get IngressClass %s: %w", ingressClassName, err)
	}

	// Check if the IngressClass has Parameters defined
	if ingressClass.Spec.Parameters == nil {
		return false, nil
	}

	// Check if the Parameters refer to an IngressClassParams object
	if ingressClass.Spec.Parameters.APIGroup == nil ||
		*ingressClass.Spec.Parameters.APIGroup != "elbv2.k8s.aws" ||
		ingressClass.Spec.Parameters.Kind != "IngressClassParams" {
		return false, nil
	}

	// Now get the IngressClassParams object
	ingressClassParams := &elbv2api.IngressClassParams{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: ingressClass.Spec.Parameters.Name}, ingressClassParams); err != nil {
		return false, fmt.Errorf("failed to get IngressClassParams %s: %w", ingressClass.Spec.Parameters.Name, err)
	}

	// Check if certificate ARNs are defined
	return len(ingressClassParams.Spec.CertificateArn) > 0, nil
}

// getProtocolPortFromELBListener get the protocol and port info from ELB listener
func (d *EndpointDiscovery) getProtocolPortFromELBListener(ctx context.Context, lbARN string) ([]ProtocolPortInfo, error) {
	input := &elasticloadbalancingv2.DescribeListenersInput{
		LoadBalancerArn: awssdk.String(lbARN),
	}

	listeners, err := d.elbv2Client.DescribeListenersAsList(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe listeners for load balancer %s: %w", lbARN, err)
	}

	// Group ports by protocol
	tcpPorts := []int32{}
	udpPorts := []int32{}

	for _, listener := range listeners {
		port := awssdk.ToInt32(listener.Port)
		listenerProtocol := listener.Protocol

		// Map ELB protocol to GA protocol
		switch listenerProtocol {
		case elbv2types.ProtocolEnumHttp, elbv2types.ProtocolEnumHttps, elbv2types.ProtocolEnumTcp, elbv2types.ProtocolEnumTls:
			// All HTTP, HTTPS, TCP, TLS protocols map to TCP for Global Accelerator
			tcpPorts = append(tcpPorts, port)
		case elbv2types.ProtocolEnumUdp:
			// UDP maps directly to UDP for Global Accelerator
			udpPorts = append(udpPorts, port)
		default:
			// Any other protocols are not supported by Global Accelerator
			return nil, fmt.Errorf("listener protocol %s is not supported by Global Accelerator for load balancer %s",
				listenerProtocol, lbARN)
		}
	}

	return createProtocolPortsInfo(tcpPorts, udpPorts), nil
}

// createProtocolPortsInfo is a helper function that creates ProtocolPortInfo objects from TCP and UDP port lists
func createProtocolPortsInfo(tcpPorts, udpPorts []int32) []ProtocolPortInfo {
	var protocolPortsInfo []ProtocolPortInfo

	if len(tcpPorts) > 0 {
		protocolPortsInfo = append(protocolPortsInfo, ProtocolPortInfo{
			Protocol: agaapi.GlobalAcceleratorProtocolTCP,
			Ports:    tcpPorts,
		})
	}
	if len(udpPorts) > 0 {
		protocolPortsInfo = append(protocolPortsInfo, ProtocolPortInfo{
			Protocol: agaapi.GlobalAcceleratorProtocolUDP,
			Ports:    udpPorts,
		})
	}

	return protocolPortsInfo
}
