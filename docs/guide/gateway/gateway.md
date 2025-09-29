# Gateway API

!!! warning
    - The team is actively trying to close conformance and support gaps.
    - Using the LBC and Gateway API together is not suggested for production workloads (yet!)


The AWS Load Balancer Controller (LBC) supports reconciliation for Kubernetes Gateway API objects. It satisfies
L4 routes (TCPRoute, UDPRoute, TLSRoute) with an AWS NLB. It satisfies L7 routes (HTTPRoute, GRPCRoute) using an AWS ALB.
Mixing protocol layers, e.g. TCPRoute and HTTPRoute on the same Gateway, is not supported.

## Current Support

The LBC Gateway API implementation supports the following Gateway API routes:

* L4 (NLBGatewayAPI): UDPRoute, TCPRoute, TLSRoute >=v2.13.3
* L7 (ALBGatewayAPI): HTTPRoute, GRPCRoute >= 2.14.0

## Prerequisites
* LBC >= v2.13.0
* For `ip` target type:
    * Pods have native AWS VPC networking configured. For more information, see the [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s#readme) documentation.
* Installation of Gateway API CRDs
    * Standard Gateway API CRDs: `kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml` [REQUIRED]
    * Experimental Gateway API CRDs: `kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/experimental-install.yaml` [OPTIONAL: Used for L4 Routes]
* Installation of LBC Gateway API specific CRDs: `kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/refs/heads/main/config/crd/gateway/gateway-crds.yaml`

## Configuration

By default, the LBC will _not_ listen to Gateway API CRDs. To enable support, specify the following feature flag(s) in the LBC deployment:

* `NLBGatewayAPI`: For enabling L4 Routing
* `ALBGatewayAPI`: For enabling L7 Routing

```
- --feature-gates=NLBGatewayAPI=true,ALBGatewayAPI=true
```

## Subnet tagging requirements
See [Subnet Discovery](../../deploy/subnet_discovery.md) for details on configuring Elastic Load Balancing for public or private placement.

## Security group
- The AWS LBC creates and attaches frontend and backend security groups to Gateway by default. For more information, please see [the security groups documentation](../../deploy/security_groups.md)

!!! tip "disable worker node security group rule management"
You can disable the worker node security group rule management using the [LoadBalancerConfiguration CRD](./loadbalancerconfig.md).

## Certificate Discovery for secure listeners

Both L4 and L7 Gateway implementations support static certificate configuration and certificate discovery using Listener hostname.
The caveat is that configuration of TLS certificates can not be done via the `certificateRefs` field of a Gateway Listener,
as the controller only supports certificate references via an ARN. In the future, we may support syncing Kubernetes secrets into ACM.


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
An [503 Fixed Response](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_FixedResponseActionConfig.html)
will be added to any Listener Rules that would have referenced the invalid backend.


