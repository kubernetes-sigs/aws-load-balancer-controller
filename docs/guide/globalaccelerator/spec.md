# API Reference

## Packages
- [aga.k8s.aws/v1beta1](#agak8sawsv1beta1)


## aga.k8s.aws/v1beta1

Package v1beta1 contains API Schema definitions for the aga v1beta1 API group

### Resource Types
- [GlobalAccelerator](#globalaccelerator)



#### ClientAffinityType

_Underlying type:_ _string_

ClientAffinityType defines the client affinity for Global Accelerator listeners.

_Validation:_
- Enum: [SOURCE_IP NONE]

_Appears in:_
- [GlobalAcceleratorListener](#globalacceleratorlistener)

| Field | Description |
| --- | --- |
| `SOURCE_IP` |  |
| `NONE` |  |


#### GlobalAccelerator



GlobalAccelerator is the Schema for the GlobalAccelerator API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `aga.k8s.aws/v1beta1` | | |
| `kind` _string_ | `GlobalAccelerator` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[GlobalAcceleratorSpec](#globalacceleratorspec)_ |  |  |  |
| `status` _[GlobalAcceleratorStatus](#globalacceleratorstatus)_ |  |  |  |


#### GlobalAcceleratorEndpoint



GlobalAcceleratorEndpoint defines an endpoint for a Global Accelerator endpoint group.



_Appears in:_
- [GlobalAcceleratorEndpointGroup](#globalacceleratorendpointgroup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[GlobalAcceleratorEndpointType](#globalacceleratorendpointtype)_ | Type specifies the type of endpoint reference. |  | Enum: [EndpointID Service Ingress Gateway] <br /> |
| `endpointID` _string_ | EndpointID is the ID of the endpoint when type is EndpointID.<br />If the endpoint is a Network Load Balancer or Application Load Balancer, this is the Amazon Resource Name (ARN) of the resource.<br />A resource must be valid and active when you add it as an endpoint.<br />Mandatory for remote regions. |  | MaxLength: 255 <br /> |
| `name` _string_ | Name is the name of the Kubernetes resource when type is Service, Ingress, or Gateway. |  |  |
| `namespace` _string_ | Namespace is the namespace of the Kubernetes resource when type is Service, Ingress, or Gateway.<br />If not specified, defaults to the same namespace as the GlobalAccelerator resource. |  |  |
| `weight` _integer_ | Weight is the weight associated with the endpoint. When you add weights to endpoints, you configure Global Accelerator to route traffic based on proportions that you specify.<br />For example, you might specify endpoint weights of 4, 5, 5, and 6 (sum=20). The result is that 4/20 of your traffic, on average, is routed to the first endpoint,<br />5/20 is routed both to the second and third endpoints, and 6/20 is routed to the last endpoint.<br />For more information, see Endpoint Weights in the AWS Global Accelerator Developer Guide:<br />https://docs.aws.amazon.com/global-accelerator/latest/dg/about-endpoints-endpoint-weights.html | 128 | Maximum: 255 <br />Minimum: 0 <br /> |
| `clientIPPreservationEnabled` _boolean_ | ClientIPPreservationEnabled indicates whether client IP address preservation is enabled for an Application Load Balancer endpoint.<br />The value is true or false. The default value is true for new accelerators.<br />If the value is set to true, the client's IP address is preserved in the X-Forwarded-For request header as traffic travels to applications on the Application Load Balancer endpoint fronted by the accelerator.<br />For more information, see Preserve Client IP Addresses in the AWS Global Accelerator Developer Guide:<br />https://docs.aws.amazon.com/global-accelerator/latest/dg/preserve-client-ip-address.html | true |  |


#### GlobalAcceleratorEndpointGroup



GlobalAcceleratorEndpointGroup defines an endpoint group for a Global Accelerator listener.



_Appears in:_
- [GlobalAcceleratorListener](#globalacceleratorlistener)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `region` _string_ | Region is the AWS Region where the endpoint group is located.<br />If unspecified, defaults to the current cluster region. |  | MaxLength: 255 <br /> |
| `trafficDialPercentage` _integer_ | TrafficDialPercentage is the percentage of traffic to send to an AWS Regions. Additional traffic is distributed to other endpoint groups for this listener<br />Use this action to increase (dial up) or decrease (dial down) traffic to a specific Region. The percentage is applied to the traffic that would otherwise have been routed to the Region based on optimal routing. | 100 | Maximum: 100 <br />Minimum: 0 <br /> |
| `portOverrides` _[PortOverride](#portoverride)_ | PortOverrides is a list of endpoint port overrides. Allows you to override the destination ports used to route traffic to an endpoint. Using a port override lets you map a list of external destination ports (that your users send traffic to) to a list of internal destination ports that you want an application endpoint to receive traffic on. |  |  |
| `endpoints` _[GlobalAcceleratorEndpoint](#globalacceleratorendpoint)_ | Endpoints is the list of endpoint configurations for this endpoint group. |  |  |


#### GlobalAcceleratorEndpointType

_Underlying type:_ _string_

GlobalAcceleratorEndpointType defines the type of endpoint for Global Accelerator.

_Validation:_
- Enum: [EndpointID Service Ingress Gateway]

_Appears in:_
- [GlobalAcceleratorEndpoint](#globalacceleratorendpoint)

| Field | Description |
| --- | --- |
| `EndpointID` |  |
| `Service` |  |
| `Ingress` |  |
| `Gateway` |  |


#### GlobalAcceleratorListener



GlobalAcceleratorListener defines a listener for the Global Accelerator.



_Appears in:_
- [GlobalAcceleratorSpec](#globalacceleratorspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `protocol` _[GlobalAcceleratorProtocol](#globalacceleratorprotocol)_ | Protocol is the protocol for the connections from clients to the accelerator.<br />When not specified, the controller will automatically determine the protocol by inspecting<br />the referenced Kubernetes resources (Service, Ingress, or Gateway) in the endpoint groups. |  | Enum: [TCP UDP] <br /> |
| `portRanges` _[PortRange](#portrange)_ | PortRanges is the list of port ranges for the connections from clients to the accelerator.<br />When not specified, the controller will automatically determine the port ranges by inspecting<br />the referenced Kubernetes resources (Service, Ingress, or Gateway) in the endpoint groups. |  | MaxItems: 10 <br />MinItems: 1 <br /> |
| `clientAffinity` _[ClientAffinityType](#clientaffinitytype)_ | ClientAffinity lets you direct all requests from a user to the same endpoint, if you have stateful applications, regardless of the port and protocol of the client request.<br />Client affinity gives you control over whether to always route each client to the same specific endpoint.<br />AWS Global Accelerator uses a consistent-flow hashing algorithm to choose the optimal endpoint for a connection.<br />If client affinity is NONE, Global Accelerator uses the "five-tuple" (5-tuple) properties—source IP address, source port, destination IP address, destination port, and protocol—to select the hash value, and then chooses the best endpoint.<br />However, with this setting, if someone uses different ports to connect to Global Accelerator, their connections might not be always routed to the same endpoint because the hash value changes.<br />If you want a given client to always be routed to the same endpoint, set client affinity to SOURCE_IP instead.<br />When you use the SOURCE_IP setting, Global Accelerator uses the "two-tuple" (2-tuple) properties— source (client) IP address and destination IP address—to select the hash value.<br />The default value is NONE. | NONE | Enum: [SOURCE_IP NONE] <br /> |
| `endpointGroups` _[GlobalAcceleratorEndpointGroup](#globalacceleratorendpointgroup)_ | EndpointGroups defines a list of endpoint groups for a Global Accelerator listener. |  |  |


#### GlobalAcceleratorProtocol

_Underlying type:_ _string_

GlobalAcceleratorProtocol defines the protocol for Global Accelerator listeners.

_Validation:_
- Enum: [TCP UDP]

_Appears in:_
- [GlobalAcceleratorListener](#globalacceleratorlistener)

| Field | Description |
| --- | --- |
| `TCP` |  |
| `UDP` |  |


#### GlobalAcceleratorSpec



GlobalAcceleratorSpec defines the desired state of GlobalAccelerator



_Appears in:_
- [GlobalAccelerator](#globalaccelerator)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Global Accelerator.<br />The name must contain only alphanumeric characters or hyphens (-), and must not begin or end with a hyphen. |  | MaxLength: 64 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z0-9_-]\{1,64\}$` <br /> |
| `ipAddresses` _string_ | IpAddresses optionally specifies the IP addresses from your own IP address pool (BYOIP) to use for the accelerator's static IP addresses.<br />You can specify one or two addresses. Do not include the /32 suffix.<br />If you bring your own IP address pool to Global Accelerator (BYOIP), you can choose an IPv4 address from your own pool to use for the accelerator's static IPv4 address.<br />After you bring an address range to AWS, it appears in your account as an address pool. When you create an accelerator, you can assign one IPv4 address from your range to it.<br />Global Accelerator assigns you a second static IPv4 address from an Amazon IP address range. If you bring two IPv4 address ranges to AWS, you can assign one IPv4 address from each range to your accelerator.<br />Note that you can't update IP addresses for an existing accelerator. To change them, you must create a new accelerator with the new addresses.<br />For more information, see Bring your own IP addresses (BYOIP) in the AWS Global Accelerator Developer Guide.<br />https://docs.aws.amazon.com/global-accelerator/latest/dg/using-byoip.html |  | MaxItems: 2 <br />MinItems: 1 <br /> |
| `ipAddressType` _[IPAddressType](#ipaddresstype)_ | IPAddressType is the value for the address type. | IPV4 | Enum: [IPV4 DUAL_STACK] <br /> |
| `tags` _map[string]string_ | Tags defines list of Tags on the Global Accelerator. |  |  |
| `listeners` _[GlobalAcceleratorListener](#globalacceleratorlistener)_ | Listeners defines the listeners for the Global Accelerator. |  |  |


#### GlobalAcceleratorStatus



GlobalAcceleratorStatus defines the observed state of GlobalAccelerator



_Appears in:_
- [GlobalAccelerator](#globalaccelerator)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | The generation observed by the GlobalAccelerator controller. |  |  |
| `acceleratorARN` _string_ | AcceleratorARN is the Amazon Resource Name (ARN) of the accelerator. |  |  |
| `dnsName` _string_ | DNSName The Domain Name System (DNS) name that Global Accelerator creates that points to an accelerator's static IPv4 addresses. |  |  |
| `dualStackDnsName` _string_ | DualStackDnsName is the Domain Name System (DNS) name that Global Accelerator creates that points to a dual-stack accelerator's four static IP addresses: two IPv4 addresses and two IPv6 addresses. |  |  |
| `ipSets` _[IPSet](#ipset) array_ | IPSets is the static IP addresses that Global Accelerator associates with the accelerator. |  |  |
| `status` _string_ | Status is the current status of the accelerator. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#condition-v1-meta) array_ | Conditions represent the current conditions of the GlobalAccelerator. |  |  |


#### IPAddressType

_Underlying type:_ _string_

IPAddressType defines the IP address type for Global Accelerator.

_Validation:_
- Enum: [IPV4 DUAL_STACK]

_Appears in:_
- [GlobalAcceleratorSpec](#globalacceleratorspec)

| Field | Description |
| --- | --- |
| `IPV4` |  |
| `DUAL_STACK` |  |


#### IPSet



IPSet is the static IP addresses that Global Accelerator associates with the accelerator.



_Appears in:_
- [GlobalAcceleratorStatus](#globalacceleratorstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ipAddresses` _string_ | IpAddresses is the array of IP addresses in the IP address set. |  |  |
| `ipAddressFamily` _string_ | IpAddressFamily is the types of IP addresses included in this IP set. |  |  |


#### PortOverride



PortOverride defines a port override for an endpoint group.
Override specific listener ports used to route traffic to endpoints that are part of an endpoint group.
For example, you can create a port override in which the listener receives user traffic on ports 80 and 443,
but your accelerator routes that traffic to ports 1080 and 1443, respectively, on the endpoints.


For more information, see Port overrides in the AWS Global Accelerator Developer Guide:
https://docs.aws.amazon.com/global-accelerator/latest/dg/about-endpoint-groups-port-override.html



_Appears in:_
- [GlobalAcceleratorEndpointGroup](#globalacceleratorendpointgroup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `listenerPort` _integer_ | ListenerPort is the listener port that you want to map to a specific endpoint port.<br />This is the port that user traffic arrives to the Global Accelerator on. |  | Maximum: 65535 <br />Minimum: 1 <br /> |
| `endpointPort` _integer_ | EndpointPort is the endpoint port that you want traffic to be routed to.<br />This is the port on the endpoint, such as the Application Load Balancer or Amazon EC2 instance. |  | Maximum: 65535 <br />Minimum: 1 <br /> |


#### PortRange



PortRange defines the port range for Global Accelerator listeners.



_Appears in:_
- [GlobalAcceleratorListener](#globalacceleratorlistener)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fromPort` _integer_ | FromPort is the first port in the range of ports, inclusive. |  | Maximum: 65535 <br />Minimum: 1 <br /> |
| `toPort` _integer_ | ToPort is the last port in the range of ports, inclusive. |  | Maximum: 65535 <br />Minimum: 1 <br /> |


