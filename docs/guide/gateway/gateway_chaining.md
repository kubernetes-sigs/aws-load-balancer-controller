## Gateway Chaining

### Introduction

Gateway chaining involves forwarding traffic from one gateway listener directly to another gateway listener. 
Specifically, the LBC allows you to configure an NLB gateway listener and point it to an ALB gateway listener.
Under the hood, this is implemented by using [ALB target of NLB](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/application-load-balancer-target.html).


Using a chaining setup provides multiple benefits:

- You can use the HTTP request-based routing features of the Application Load Balancer in combination with features that the Network Load Balancer supports.
- Use of endpoint services (AWS PrivateLink)
- Static IP for Application Load Balancer.
- Serve TCP and HTTP traffic from a single endpoint.

### Set up

This guide will walk you through setting up a chained Gateway.


#### ALB Setup

In the YAML below, we set up an ALB Gateway with an HTTP listener on port 80 and an HTTPS listener on port 443. These listeners forward traffic to
an arbitrary backend service. It's important to note that the ALB Gateway is configured as an internal load balancer. Clients
that wish to connect to the ALB Gateway must do so via the NLB Gateway we will set up next. While it's possible
to use an internet-facing ALB Gateway where clients could communicate directly, in a chained
setup the NLB Gateway always uses private IP addresses to communicate with the ALB Gateway.

```yaml
# alb-gatewayclass.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: aws-alb-gateway-class
spec:
  controllerName: gateway.k8s.aws/alb
---
# my-alb-gateway.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-alb-gateway
  namespace: example-ns
spec:
  gatewayClassName: aws-alb-gateway-class
  infrastructure:
    parametersRef:
      kind: LoadBalancerConfiguration
      name: alb-lb-config
      group: gateway.k8s.aws
  listeners:
  - name: http
    protocol: HTTP
    port: 80
    allowedRoutes:
      namespaces:
        from: Same
  - name: https
    protocol: HTTPS
    port: 443
    allowedRoutes:
      namespaces:
        from: Same
---
# lbconfig.yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: alb-lb-config
  namespace: example-ns
spec:
  listenerConfigurations:
    - protocolPort: HTTPS:443
      defaultCertificate: <my cert arn>
---
# httproute.yaml
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
    - name: echoserver
      port: 80
```

#### NLB Setup

In the YAML below, we set up an NLB Gateway with TCP listeners on ports 80 and 443. These listeners forward traffic to
the ALB Gateway configured above. The NLB Gateway is configured as internet-facing to allow external clients
to connect. The NLB will route traffic to the internal ALB using private IP addresses.

```yaml
# nlb-gatewayclass.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: aws-nlb-gateway-class
spec:
  controllerName: gateway.k8s.aws/nlb
---
# my-nlb-gateway.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-tcp-gateway
  namespace: example-ns
spec:
  gatewayClassName: aws-nlb-gateway-class
  infrastructure:
    parametersRef:
      group: gateway.k8s.aws
      kind: LoadBalancerConfiguration
      name: nlb-lb-config
  listeners:
  - name: unsecure
    protocol: TCP
    port: 80
    allowedRoutes:
      namespaces:
        from: Same
  - name: secure
    protocol: TCP
    port: 443
    allowedRoutes:
      namespaces:
        from: Same
---
# lbconfig.yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: nlb-lb-config
  namespace: example-ns
spec:
  scheme: internet-facing
---
# my-unsecure-tcproute.yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: my-unsecure-app-route
  namespace: example-ns
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-tcp-gateway
    sectionName: unsecure
  rules:
  - backendRefs:
    - name: my-alb-gateway
      kind: Gateway
      port: 80
---
# my-secure-tcproute.yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: my-secure-app-route
  namespace: example-ns
spec:
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: my-tcp-gateway
      sectionName: secure
  rules:
    - backendRefs:
        - name: my-alb-gateway
          kind: Gateway
          port: 443
---
# tg-configuration.yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: TargetGroupConfiguration
metadata:
  name: example-tg-config
  namespace: example-ns
spec:
  targetReference:
    name: my-alb-gateway
    kind: Gateway
  routeConfigurations:
    - routeIdentifier:
        kind: TCPRoute
        namespace: example-ns
        name: my-unsecure-app-route
      targetGroupProps:
        healthCheckConfig:  
          healthCheckProtocol: HTTP
    - routeIdentifier:
        kind: TCPRoute
        namespace: example-ns
        name: my-secure-app-route
      targetGroupProps:
        healthCheckConfig:  
          healthCheckProtocol: HTTPS
```

#### Customizing the ALB Gateway target settings

Customizing the ALB Gateway, in the context as a target, works exactly the same way as customizing a Target Group
based on a Kubernetes Service. The only caveat is that target groups of type ALB do not support attribute customization,
this is an AWS limitation and not one imposed within the controller. For more information about customization, see the
[TargetGroupConfiguration CRD documentation](./targetgroupconfig.md).

In the example presented above, we have customized the target group that points to the ALB listener port on 443. In our example,
this is required when forwarding traffic from the NLB to the ALB on listener port 443 as the ALB listener is expecting HTTPS traffic; even
for health check traffic.

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: TargetGroupConfiguration
metadata:
  name: example-tg-config
  namespace: example-ns
spec:
  targetReference:
    name: my-alb-gateway
    kind: Gateway
  routeConfigurations:
    - routeIdentifier:
        kind: TCPRoute
        namespace: example-ns
        name: my-unsecure-app-route
      targetGroupProps:
        healthCheckConfig:  
          healthCheckProtocol: HTTP
    - routeIdentifier:
        kind: TCPRoute
        namespace: example-ns
        name: my-secure-app-route
      targetGroupProps:
        healthCheckConfig:  
          healthCheckProtocol: HTTPS
```

#### Cross namespace access

Chained Gateways support [Reference Grants](https://gateway-api.sigs.k8s.io/api-types/referencegrant/) to support chaining
Gateways in different namespaces. The Reference Grant must exist within the namespace of the ALB Gateway. The same semantics used
for routes and reference grants apply to Gateway-based reference grants.

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: example-reference-grant
  namespace: alb-gw-ns
spec:
  from:
  - group: gateway.networking.k8s.io
    kind: TCPRoute
    namespace: nlb-gw-ns
  to:
  - group: gateway.networking.k8s.io
    kind: Gateway
```

In this example, we are establishing a reference grant that allows TCPRoutes from the `nlb-gw-ns` namespace
to attach any ALB Gateway in the `alb-gw-ns` namespace.
