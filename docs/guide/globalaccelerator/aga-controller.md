# AWS Global Accelerator Controller

## Introduction

AWS Global Accelerator is a networking service that improves the performance of users' traffic by up to 60% using the AWS global network. The AWS Load Balancer Controller (LBC) extends its capabilities with a new AWS Global Accelerator Controller that allows users to declaratively manage accelerators, listeners, and endpoint groups using Kubernetes Custom Resource Definitions (CRDs).

This feature bridges a significant operational gap - traditionally, accelerators must be manually created and managed through the AWS console, CLI, or CloudFormation. With the AWS Global Accelerator Controller, accelerators are managed directly by your Kubernetes cluster, eliminating operational overhead and integrating with your existing Kubernetes workflows.

## Architecture

The AWS Global Accelerator Controller operates as a continuous reconciliation loop within the AWS Load Balancer Controller, watching for changes to the `GlobalAccelerator` CRD. When it detects a change, it:

1. Translates the desired state defined in the CRD into corresponding AWS Global Accelerator API calls
2. Automatically discovers resources like Elastic Load Balancers (ELBs) referenced by Kubernetes Services, Ingresses, and Gateway APIs, or directly manages ELBs specified by their ARNs (endpointID)
3. Maintains and updates the status of the AWS Global Accelerator resources in the CRD
4. Handles cleanup and resource deletion when objects are removed

## Key Features

### Monolithic CRD Design

The controller uses a single `GlobalAccelerator` resource to manage the entire AWS Global Accelerator hierarchy, including:

- The accelerator itself
- Listeners with protocol and port range configurations
- Endpoint groups with region and traffic dial settings
- Endpoints from different Kubernetes resource types (Service, Ingress, and Gateway API resources managed by AWS Load Balancer Controller or ELBs referenced by their ARNs)

This design simplifies management by keeping all configuration in one place while still allowing granular control over each component.

### Full Lifecycle Management (CRUD)

The controller manages the complete lifecycle of AWS Global Accelerator resources:

- **Create**: Provisions new AWS Global Accelerator resources from CRD specifications
- **Read**: Updates CRD status with current state and ARNs from AWS
- **Update**: Modifies existing accelerator configurations when the CRD is updated
- **Delete**: Cleans up AWS resources when the CRD is deleted

### Automatic Endpoint Discovery

The AWS Global Accelerator Controller provides comprehensive auto-discovery capabilities that simplify Global Accelerator configuration in Kubernetes:

#### Resource Discovery
The controller automatically discovers load balancers from multiple Kubernetes resource types in the local cluster:

- **Service resources**: Discovers Network Load Balancers (NLBs) from Service type LoadBalancer
- **Ingress resources**: Discovers Application Load Balancers (ALBs) from Ingress resources
- **Gateway API resources**: Discovers ALBs and NLBs from Gateway API resources

#### Automatic Configuration
For simple use cases, the controller can automatically configure listener protocol and port ranges based on the discovered endpoints or the ELB ARN:

```yaml
kind: GlobalAccelerator
metadata:
  name: autodiscovery-accelerator
  namespace: test-ns
spec:
  name: "autodiscovery-test-accelerator"
  listeners:
    - # Protocol and portRanges will be auto-discovered from the endpoint
      endpointGroups:
        - endpoints:
            - type: Ingress
              name: web-ingress
              weight: 200
```

### Manual Endpoint Registration

For multi-region configurations or when referencing load balancers, you can manually specify endpoint ARNs instead of using auto-discovery:

```yaml
spec:
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 443
          toPort: 443
      endpointGroups:
        - endpoints:
            - type: EndpointID
              endpointID: arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-load-balancer/1234567890123456
```

> **Note:** While auto-discovery currently only works within the same AWS region as the controller, you can manually specify endpoints in other regions by explicitly setting the region field and providing the endpoint ARN. For cross-region configurations, you must manually specify the protocol and port ranges as well:
>
> ```yaml
> listeners:
>   - protocol: TCP  # Must explicitly specify protocol
>     portRanges:    # Must explicitly specify port ranges
>       - fromPort: 443
>         toPort: 443
>     endpointGroups:
>       - region: us-west-2  # Explicitly set a different region
>         endpoints:
>           - type: EndpointID
>             endpointID: arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-load-balancer/1234567890123456
> ```

### BYOIP (Bring Your Own IP) Support

The AWS Global Accelerator Controller supports Bring Your Own IP (BYOIP) functionality, which allows you to use your own IP address ranges with AWS Global Accelerator.

#### Prerequisites

To use your own IP address range with Global Accelerator, review the requirements, and then follow the steps provided in the [Bring your own IP addresses (BYOIP) in AWS Global Accelerator](https://docs.aws.amazon.com/global-accelerator/latest/dg/using-byoip.html) documentation.

#### Controller Limitations

The AWS Global Accelerator Controller has the following limitations when working with BYOIP addresses:

1. **Creation-Only Feature**: BYOIP addresses can only be specified during the initial creation of an accelerator.

2. **No Updates Allowed**: This is a controller limitation - IP addresses cannot be updated after an accelerator has been created. If you need to change IP addresses, you must create a new accelerator.

3. **Handling Update Attempts**: If users attempt to modify IP addresses on an existing accelerator, the controller will log this as an invalid operation and ignore the IP address changes, preserving the original configuration.

#### Configuration Example

For BYOIP usage, specify the addresses in the `ipAddresses` field of your GlobalAccelerator resource:

```yaml
spec:
  ipAddressType: IPV4
  ipAddresses:
    - "198.51.100.10"  # Your own IP from BYOIP pool
```

### Status Reporting

The controller updates the CRD status with important information from the AWS Global Accelerator:

- The Accelerator's ARN for reference in other systems
- The default DNS name for the accelerator
- The dual-stack DNS name when available for IPv6 support
- IP address sets for both IPv4 and dual-stack configurations
- The current state of the accelerator (deployed, in progress, etc.)
- Conditions reflecting the health and status of the reconciliation process

#### Accelerator Status States

The `status.status` field in the GlobalAccelerator CRD reflects the current state of the accelerator in AWS. This field can have the following values:

- **DEPLOYED**: The accelerator is fully deployed and operational
- **IN_PROGRESS**: The accelerator is being created or updated
- **FAILED**: The accelerator creation or update has failed

When an accelerator is first created, it will typically show as IN_PROGRESS and then transition to DEPLOYED once fully provisioned. During updates, it may temporarily show as IN_PROGRESS again.

If the status shows FAILED, check the controller logs and the `status.conditions` field for more detailed error information.

## Intelligent Listener Management

The AWS Global Accelerator Controller intelligently manages listeners to minimize service disruption during reconciliation. The controller:

1. Preserves existing listeners when possible to maintain stable ARNs and associated resources
2. Manages conflicts carefully to ensure smooth transitions during updates
3. Optimizes the operation order to minimize downtime and maintain traffic flow

This approach ensures reliable service while making necessary changes to your Global Accelerator configuration.

## Port Overrides

Port overrides allow you to map traffic from a specific listener port to a different port on your endpoint. This is especially useful when you need to support both direct access to your endpoint and access through AWS Global Accelerator.

For example, your application might be accessible directly on port 80, but you want to route traffic from AWS Global Accelerator's listener port 443 to this endpoint. Port overrides enable this flexibility.

```yaml
endpointGroups:
  - trafficDialPercentage: 100
    portOverrides:
      - listenerPort: 443     # The port your Global Accelerator listener is using
        endpointPort: 80      # The port your endpoint is listening on
```

For more information about when and how to use port overrides, see [AWS Global Accelerator Port Overrides](https://docs.aws.amazon.com/global-accelerator/latest/dg/about-endpoint-groups-port-override.html) in the AWS documentation.

!!!note "Note"
    The AWS Global Accelerator Controller handles all port override constraints automatically, ensuring your configuration is valid.

## Cross-Namespace Endpoint References


The AWS Global Accelerator controller supports cross-namespace references for endpoints using the [Gateway API ReferenceGrant](https://gateway-api.sigs.k8s.io/api-types/referencegrant/) approach, which is a common pattern for secure cross-namespace references in Kubernetes. Cross-namespace references allow a GlobalAccelerator resource in one namespace (e.g., `accelerator-ns`) to reference resources in another namespace (e.g., `web-ns`), provided that a ReferenceGrant exists in the target namespace explicitly allowing the reference.

### Using Cross-Namespace References

#### Step 1: Create a ReferenceGrant in the Target Namespace

To allow a GlobalAccelerator in namespace A to reference a resource in namespace B, create a ReferenceGrant in namespace B:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-accelerator-references
  namespace: web-ns  # Target namespace containing the service
spec:
  from:
  - group: aga.k8s.aws
    kind: GlobalAccelerator
    namespace: accelerator-ns  # Source namespace containing the GlobalAccelerator
  to:
  - group: ""
    kind: Service
    name: web-service  
```

#### Step 2: Reference the Resource in your GlobalAccelerator

Once the ReferenceGrant is in place, you can reference the resource in your GlobalAccelerator:

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: my-accelerator
  namespace: accelerator-ns  # Source namespace
spec:
  listeners:
  - endpointGroups:
    - endpoints:
      - namespace: web-ns  # Target namespace
        name: web-service  # Service in target namespace
        type: Service
```

!!!note "To use cross-namespace references"

    1. The Gateway API CRDs must be installed in your cluster (specifically the ReferenceGrant CRD)
    2. The controller must be granted permission to read ReferenceGrants cluster-wide

## Sample CRDs

### Basic Global Accelerator with Single TCP Port

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: simple-accelerator
  namespace: default
spec:
  name: "simple-accelerator"
  ipAddressType: IPV4
  tags:
    Environment: "production"
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
      clientAffinity: NONE
```

### Multiple Port Ranges with Source IP Affinity

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: multi-port-accelerator
  namespace: default
spec:
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      clientAffinity: SOURCE_IP
```

### Complex Configuration with Multiple Listeners and Endpoints

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: complex-accelerator
  namespace: default
spec:
  ipAddressType: IPV4
  tags:
    Environment: "test"
    Team: "platform"
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      clientAffinity: SOURCE_IP
      endpointGroups:
        - trafficDialPercentage: 100
          portOverrides:
            - listenerPort: 80
              endpointPort: 8081
          endpoints:
            - type: Service
              name: service-nlb-1
              weight: 128
              clientIPPreservationEnabled: true
            - type: Ingress
              name: test-ingress
              weight: 240
    - protocol: UDP
      portRanges:
        - fromPort: 53
          toPort: 53
      clientAffinity: NONE
```

## Troubleshooting

Common issues and their solutions:

### Status Shows Failure

Check the controller logs for detailed error messages:

```bash
kubectl logs -n kube-system -l app.kubernetes.io/name=aws-load-balancer-controller
```

### Endpoint Discovery Not Working

Ensure your Service/Ingress/Gateway resources:

1. Are properly configured with correct annotations
2. Have been successfully provisioned with actual AWS load balancers
3. Are in the same namespace as specified in the endpoint


## References

- [AWS Global Accelerator Developer Guide](https://docs.aws.amazon.com/global-accelerator/latest/dg/what-is-global-accelerator.html)
- [GlobalAccelerator CRD Reference](spec.md)
- [AWS Load Balancer Controller Installation](../../deploy/installation.md)
