# Gateway API

The AWS Load Balancer Controller (LBC) supports reconciliation for Kubernetes Gateway API objects. It satisfies
L4 routes (TCPRoute, UDPRoute, TLSRoute) with an AWS NLB. It satisfies L7 routes (HTTPRoute, GRPCRoute) using an AWS ALB.
Mixing protocol layers, e.g. TCPRoute and HTTPRoute on the same Gateway, is not supported.

## Current Support

The LBC Gateway API implementation supports the following Gateway API routes:

* L4 (NLBGatewayAPI): UDPRoute, TCPRoute, TLSRoute >=v2.13.3
* L7 (ALBGatewayAPI): HTTPRoute, GRPCRoute >= 2.14.0

The LBC is built for Gateway API version v1.3.0.

## Prerequisites
* LBC >= v2.13.0
* For `ip` target type:
    * Pods have native AWS VPC networking configured. For more information, see the [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s#readme) documentation.
* Installation of Gateway API CRDs
    * Standard Gateway API CRDs: `kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml` [REQUIRED]
    * Experimental Gateway API CRDs: `kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/experimental-install.yaml` [OPTIONAL: Used for L4 Routes]
* Installation of LBC Gateway API specific CRDs: `kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/refs/heads/main/config/crd/gateway/gateway-crds.yaml`

## Configuration

The Load Balancer Controller (LBC) will attempt to detect Gateway CRDs. 
If they are present, the respective controller will be enabled. 
To explicitly disable these controllers, use the following feature gates:

```--feature-gates=NLBGatewayAPI=false,ALBGatewayAPI=false```

For the NLB Gateway controller (Layer 4) to be enabled, ensure the following CRDs are installed:
`Gateway`, `GatewayClass`, `TCPRoute`, `UDPRoute`, and `TLSRoute`

For the ALB Gateway controller (Layer 7) to be enabled, ensure the following CRDs are installed:
`Gateway`, `GatewayClass`, `HTTPRoute`, and `GRPCRoute`

## Subnet tagging requirements
See [Subnet Discovery](../../deploy/subnet_discovery.md) for details on configuring Elastic Load Balancing for public or private placement.

## Security group
- The AWS LBC creates and attaches frontend and backend security groups to Gateway by default. For more information, please see [the security groups documentation](../../deploy/security_groups.md)

!!! tip "disable worker node security group rule management"
You can disable the worker node security group rule management using the [LoadBalancerConfiguration CRD](./loadbalancerconfig.md).

## Certificate Discovery for secure listeners

Both L4 and L7 Gateway implementations support static certificate configuration and certificate discovery
using the hostname field on the Gateway listener and attached routes.
See the Gateway API [documentation](https://gateway-api.sigs.k8s.io/reference/spec/#httproutespec)
for more information on how specifying hostnames at listener and route level work with each other.
An important caveat to consider is
that configuration of TLS certificates cannot be done via the `certificateRefs` field of a Gateway Listener.
In the future, we may support syncing Kubernetes secrets into ACM.


### Worker node security groups selection
The controller automatically selects the worker node security groups that it modifies to allow inbound traffic using the following rules:

* For `instance` mode, the security group of each backend worker node's primary elastic network interface (ENI) is selected.
* For `ip` mode, the security group of each backend pod's ENI is selected.

!!! important "Multiple security groups on an ENI"

    If there are multiple security groups attached to an ENI, the controller expects only one security group tagged with following tags:

    | Key                                     | Value               |
    | --------------------------------------- | ------------------- |
    | `kubernetes.io/cluster/${cluster-name}` | `owned` or `shared` |

    `${cluster-name}` is the name of the Kubernetes cluster.

If it is possible for multiple security groups with the tag `kubernetes.io/cluster/${cluster-name}` to be on a target ENI, you may use the `--service-target-eni-security-group-tags` flag to specify additional tags that must also match in order for a security group to be used.


## Misconfigured Services

The L4 and L7 gateways handle misconfigured services differently. 


### L4

```
# my-tcproute.yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: TCPRoute
metadata:
  name: my-tcp-app-route
  namespace: example-ns
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-tcp-gateway
    sectionName: tcp-app # Refers to the specific listener on the Gateway
  rules:
  - backendRefs:
    - name: my-tcp-service # Kubernetes Service
      port: 9000
```

When `my-tcp-service` or the configured service port can't be found,
the target group will not be materialized on any NLBs that the route attaches to.


### L7

```
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: my-http-app-route
  namespace: example-ns
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-alb-gateway
    sectionName: http
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-alb-gateway
    sectionName: https
  rules:
  - backendRefs:
    - name: my-http-service
      port: 9000
```

When `my-http-service` or the configured service port can't be found,
the target group will not be materialized on any ALBs that the route attaches to.
A [500 Fixed Response](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_FixedResponseActionConfig.html)
will be added to any Listener Rules that would have referenced the invalid backend.

## Specify out-of-band Target Groups

Use an existing AWS Target Group with a Gateway-managed Load Balancer.
This lets you integrate or migrate legacy applications that are already
registered with an AWS Target Group outside the controller's lifecycle.

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: tcproute
  namespace: example-ns
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: nlb-gw
    sectionName: tls
  rules:
  - backendRefs:
    - group: ""
      kind: TargetGroupName
      name: test-gw-import123
      weight: 1
```

This support exists for all route types managed by the controller.




