# AWS Global Accelerator Controller Examples

This document provides practical examples for using the AWS Global Accelerator Controller feature of the AWS Load Balancer Controller in various scenarios.

## Basic Examples

### Single Ingress Acceleration

This example creates a Global Accelerator that accelerates traffic to a single ingress resource. It's the simplest configuration and ideal for getting started.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: web-app-accelerator
  namespace: web-app
spec:
  name: "web-app-accelerator"
  ipAddressType: IPV4
  tags:
    Environment: "production"
    Application: "web-app"
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      clientAffinity: NONE
      endpointGroups:
        - endpoints:
            - type: Ingress
              name: web-app-ingress
              namespace: web-app
```

### Network Load Balancer Service Acceleration

This example accelerates traffic to a Network Load Balancer provisioned by a Kubernetes Service of type LoadBalancer.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: api-service-accelerator
  namespace: api
spec:
  name: "api-service-accelerator"
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 443
          toPort: 443
      clientAffinity: SOURCE_IP
      endpointGroups:
        - endpoints:
            - type: Service
              name: api-service
              weight: 128
              clientIPPreservationEnabled: true
```

### Gateway API Acceleration

This example accelerates traffic to a Gateway API resource (requires Gateway API CRDs installed in your cluster).

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: gateway-accelerator
  namespace: gateway-ns
spec:
  name: "gateway-accelerator"
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      endpointGroups:
        - endpoints:
            - type: Gateway
              name: my-gateway
              weight: 128
```

### Auto-Discovery Configuration

This minimal configuration uses the auto-discovery feature to determine protocol and port ranges from the ingress resource.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: autodiscovery-accelerator
  namespace: default
spec:
  name: "autodiscovery-accelerator"
  listeners:
    - endpointGroups:
        - endpoints:
            - type: Ingress
              name: web-ingress
              namespace: default
              weight: 200
```

## Advanced Examples

### Multiple Listeners with Different Protocols

This example creates a Global Accelerator with both TCP and UDP listeners for different services.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: multi-protocol-accelerator
  namespace: default
spec:
  name: "multi-protocol-accelerator"
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      clientAffinity: SOURCE_IP
      endpointGroups:
        - endpoints:
            - type: Ingress
              name: web-ingress
    - protocol: UDP
      portRanges:
        - fromPort: 53
          toPort: 53
      clientAffinity: NONE
      endpointGroups:
        - endpoints:
            - type: Service
              name: dns-service
```

### Traffic Distribution with Multiple Endpoints

This example distributes traffic between multiple endpoints with different weights.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: traffic-distribution-accelerator
  namespace: default
spec:
  name: "traffic-distribution-accelerator"
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
      endpointGroups:
        - endpoints:
            - type: Service
              name: service-1
              weight: 200  # Higher weight - receives more traffic
            - type: Service
              name: service-2
              weight: 100  # Lower weight - receives less traffic
```

### Port Override Example

This example demonstrates port overrides to map external ports to different internal ports.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: port-override-accelerator
  namespace: default
spec:
  name: "port-override-accelerator"
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      endpointGroups:
        - portOverrides:
            - listenerPort: 80
              endpointPort: 8080  # Redirects traffic from port 80 to port 8080
            - listenerPort: 443
              endpointPort: 8443  # Redirects traffic from port 443 to port 8443
          endpoints:
            - type: Service
              name: backend-service
```

### Cross-Region Manual Endpoint

This example uses manual endpoint registration with ARNs for cross-region configurations.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: cross-region-accelerator
  namespace: default
spec:
  name: "cross-region-accelerator"
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 443
          toPort: 443
      endpointGroups:
        # Local region endpoint group
        - endpoints:
            - type: Service
              name: local-service
        # Remote region endpoint group
        - region: us-west-2  # Specific AWS region
          trafficDialPercentage: 50  # Split traffic 50%
          endpoints:
            - type: EndpointID
              endpointID: arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/remote-lb/1234567890123456
              weight: 128
```

### BYOIP (Bring Your Own IP) Configuration

This example demonstrates using your own IP addresses with Global Accelerator.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: byoip-accelerator
  namespace: default
spec:
  name: "byoip-accelerator"
  ipAddressType: IPV4
  ipAddresses:
    - "198.51.100.10"  # Your own IP from BYOIP pool
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 443
          toPort: 443
      endpointGroups:
        - endpoints:
            - type: Ingress
              name: secure-ingress
```

### Dual-Stack (IPv4 and IPv6) Configuration

This example sets up a dual-stack Global Accelerator that supports both IPv4 and IPv6.

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: dual-stack-accelerator
  namespace: default
spec:
  name: "dual-stack-accelerator"
  ipAddressType: DUAL_STACK  # Support both IPv4 and IPv6
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      endpointGroups:
        - endpoints:
            - type: Service
              name: dual-stack-service
```

## Important Limitations and Best Practices

### Cross-Namespace References

In the initial release, the AWS Global Accelerator Controller has a limitation regarding cross-namespace references:

1. **Same-Namespace Default**: By default, the controller expects endpoints to be in the same namespace as the GlobalAccelerator resource.

2. **Security Considerations**: Cross-namespace references without proper security controls can present security risks.

### BYOIP Considerations

When using Bring Your Own IP (BYOIP) with Global Accelerator:

1. **Creation-Only**: IP addresses can only be set during initial creation and cannot be changed afterward.

2. **New Accelerator Required**: If you need to change IP addresses, you must create a new GlobalAccelerator resource.
