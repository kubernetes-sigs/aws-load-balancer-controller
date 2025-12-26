# QUIC Protocol Support

The AWS Load Balancer Controller supports QUIC protocol for HTTP/3 traffic on Network Load Balancers through the Gateway API.

## Overview

QUIC (Quick UDP Internet Connections) is a transport protocol that provides improved performance and reduced latency compared to traditional TCP-based protocols. It's the foundation for HTTP/3.

## Requirements

- Network Load Balancer (NLB) only
- IP target type (instance target type not supported)
- UDP or TCP_UDP protocol listeners
- AWS Load Balancer Controller >= v2.13.0

## Configuration

QUIC support is enabled through the `quicEnabled` field in the [LoadBalancerConfiguration](./loadbalancerconfig.md#quicenabled).

### Basic QUIC Example

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: GatewayClass
metadata:
  name: aws-nlb-gateway-class
spec:
  controllerName: gateway.k8s.aws/nlb
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: quic-gateway
  namespace: default
spec:
  gatewayClassName: aws-nlb-gateway-class
  listeners:
  - name: udp-listener
    port: 80
    protocol: UDP
  - name: tcp-udp-listener
    port: 443
    protocol: TCP_UDP
---
apiVersion: gateway.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: quic-config
  namespace: default
spec:
  listenerConfigurations:
  - protocolPort: "UDP:80"
    quicEnabled: true      # UDP becomes QUIC
  - protocolPort: "TCP_UDP:443"
    quicEnabled: true      # TCP_UDP becomes TCP_QUIC
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: UDPRoute
metadata:
  name: quic-route
  namespace: default
spec:
  parentRefs:
  - name: quic-gateway
    sectionName: udp-listener
  rules:
  - backendRefs:
    - name: quic-service
      port: 8080
```

## Protocol Upgrade Behavior

When `quicEnabled: true` is set:

| Original Protocol | Upgraded Protocol | Description |
|-------------------|-------------------|-------------|
| UDP | QUIC | Pure QUIC traffic for HTTP/3 |
| TCP_UDP | TCP_QUIC | Dual protocol supporting both TCP and QUIC |

## Limitations

- QUIC is only supported on Network Load Balancers
- Target groups must use IP target type
- Instance target type is not supported with QUIC protocols
- Only UDP and TCP_UDP listeners can be upgraded to QUIC

## Benefits

- **Reduced Latency**: QUIC reduces connection establishment time
- **Improved Performance**: Better handling of packet loss and network changes
- **HTTP/3 Support**: Native support for the latest HTTP protocol version
- **Connection Migration**: Maintains connections during network changes
