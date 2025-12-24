# Gateway API for Layer 7 (ALB) Implementation

This section details the **AWS Load Balancer Controller's (LBC)** architecture and operational flow when processing Gateway API resources for Layer 7 traffic utilizing an AWS ALB.

### Gateway API Resources and Controller Architecture

The LBC implements Gateway API support through a dual architecture, using distinct controller instances for **Layer 4 (L4)** and **Layer 7 (L7)** routing. This design allows for specialized and optimized reconciliation aligned with the underlying AWS Load Balancer capabilities.

The LBC instance dedicated to L7 routing monitors the following Gateway API resources:

* **`GatewayClass`**: For L7 routing, the LBC specifically manages `GatewayClass` resources with the `controllerName` set to `gateway.k8s.aws/alb`.
* **`Gateway`**: For every gateway that references a `GatewayClass` with the `controllerName` set to `gateway.k8s.aws/alb`, the LBC provisions an AWS ALB.
* **`HTTPRoute`**: Defines HTTP-specific routing rules, enabling HTTP communication between the load balancer and backend targets. These routes are handled by an **AWS ALB**.
* **`GRPCRoute`**: Defines GRPC-specific routing rules, enabling GRPC communication between the load balancer and backend targets. These routes are handled by an **AWS ALB**.
* **`ReferenceGrant`**: Defines cross-namespace access. For more information, [see](https://gateway-api.sigs.k8s.io/api-types/referencegrant/)
* **`LoadBalancerConfiguration` (LBC CRD)**: A Custom Resource Definition utilized for fine-grained customization of the provisioned ALB. This CRD can be attached to a `Gateway` or its `GatewayClass`. For more information, please refer to  [How customization works](customization.md#customizing-the-gateway-load-balancer-using-loadbalancerconfiguration-crd).
* **`TargetGroupConfiguration` (LBC CRD)**: A Custom Resource Definition used for service-specific customizations of AWS Target Groups. This CRD is associated with a Kubernetes `Service`. For more information, please refer to [How customization works](customization.md#customizing-services-target-groups-using-targetgroupconfiguration-crd).
* **`ListenerRuleConfiguration` (LBC CRD)**: A Custom Resource Definition used for rule-specific customizations of AWS Listener Rules. This CRD is associated with one or more `HTTPRoute` or `GRPCRoute` to provide functionality supported by AWS ALB, but not natively available within the Gateway API. For more information, please refer to [Advanced Configuration](customization.md#customizing-l7-routing-rules).

### The Reconciliation Loop

The LBC operates on a continuous **reconciliation loop** within your cluster to maintain the desired state of AWS Load Balancer resources:

1.  **Event Watching:** The L7-specific controller instance constantly monitors the Kubernetes API for changes to the resources mentioned above related to ALB provisioning.
2.  **Queueing:** Upon detecting any modification, creation, or deletion of these resources, the respective object is added to an internal processing queue.
3.  **Processing:**
    * The controller retrieves the resource from the queue.
    * It validates the resource's configuration and determines if it falls under its management (e.g., by checking the `GatewayClass`'s `controllerName`). If it does, it enqueues the relevant gateway for processing.
    * The Kubernetes Gateway API definitions are then translated into an equivalent desired state within AWS (e.g., specific ALB, Listeners, Listener Rules, Target Groups, Addons, etc).
    * This desired state is compared against the actual state of AWS resources.
    * Necessary AWS API calls are executed to reconcile any identified discrepancies, ensuring the cloud infrastructure matches the Kubernetes declaration.
4.  **Status Updates:** After reconciliation, the LBC updates the `status` field of the Gateway API resources in Kubernetes. This provides real-time feedback on the provisioned AWS resources, such as the ALB's DNS name and ARN, and whether the gateways are accepted.

### Step-by-Step L7 Gateway API Resource Implementation with an Example

This section illustrates the step-by-step process of configuring the Gateway API resources, demonstrating provisioning and configuration for an ALB.

Consider a scenario where an application exposes an HTTP and HTTPS endpoint.

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
      name: test-gw-lbconfig-1
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
  name: test-gw-lbconfig-1
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
    - name: <your service>
      port: <your service port>
```

* **API Event Detection:** The LBC's L7 controller continuously monitors the Kubernetes API server. Upon detecting the `aws-alb-gateway-class` (with `controllerName: gateway.k8s.aws/alb`), the `my-alb-gateway` (referencing this `GatewayClass`), and `my-http-app-route` (referencing `my-alb-gateway`'s listener) resources, it recognizes its responsibility to manage these objects and initiates the provisioning of AWS resources.
* **ALB Provisioning:** An **AWS Application Load Balancer (ALB)** is provisioned in AWS for the `my-alb-gateway` resource with default settings. At this stage, the ALB is active but does not yet have any configured listeners. As soon as the ALB becomes active, the status of the gateway is updated.
* **L7 Listener Materialization:** The controller processes the `my-http-app-route` resource. Given that the `HTTPRoute` validly references the `my-alb-gateway` and its `http` and `https` listeners, two **Listeners** are materialized on the ALB. The listeners accept HTTP traffic on port 80 and HTTPS traffic on port 443 and forward them to the service hosted on the HTTPRoute.
* **Target Group Creation:** An **AWS Target Group** is created that contains the cluster nodes with the configured NodePort of the service.

#### Feature Comparison: ALB Gateways vs. Standard Gateway API

The AWS ALB Gateway implementation does not fully implement all Core functionality as mandated by the Gateway API. This is due
to feature limitations of AWS ALB, this table should be helpful in navigating the features available within the ALB Gateway. For more
information see the [Gateway API Conformance Page](https://gateway-api.sigs.k8s.io/concepts/conformance/)

##### GatewayClass

| Field               | Conformance Level | ALB Gateway Support |
|:--------------------|:------------------|--------------------:|
| ControllerName      | Core              |                   ✅ |
| ParametersRef       | Core              |                   ✅ |
| Description         | Core              |                   ✅ |
| Status              | Core              |                   ✅ |

##### Gateway

| Field               | Conformance Level |           ALB Gateway Support |
|:--------------------|:------------------|------------------------------:|
| Listeners           | Core              |                             ✅ |
| Addresses           | Core              |                             ✅ |
| Infrastructure      | Core              | ✅ -- Used to attach LB Config |
| Status              | Core              |    ✅ -- Find the ALB ARN here |
| AllowedListeners    | Experimental      |                             ❌ |
| GatewayTLSConfig    | Extended          |                             ❌ |
| GatewayDefaultScope | Core              |                             ❌ |

##### Listener

| Field                               | Conformance Level | ALB Gateway Support |
|:------------------------------------|:------------------|--------------------:|
| Protocol Specification              | Core              |                   ✅ |
| Port Specification                  | Core              |                   ✅ |
| Section Specification               | Core              |                   ✅ |
| Hostname Specification              | Core              |                   ✅ |
| Allowed Routes Specification        | Core              |                   ✅ |
| ListenerTLSConfig - TLSModeType     | Core              |                   ✅ |
| ListenerTLSConfig - CertificateRefs | Core              |  ❌ -- Use LB Config |
| ListenerTLSConfig - Options         | Core              |  ❌ -- Use LB Config |


##### GRPCRoute

| Field                                                    | Conformance Level |                                                                                                 ALB Gateway Support |
|:---------------------------------------------------------|:------------------|--------------------------------------------------------------------------------------------------------------------:|
| ParentRefs                                               | Core              |                                                                                                                   ✅ |
| UseDefaultGateways                                       | Core              |                                                                                                                   ❌ |
| Hostnames                                                | Core              |                                                                                                                   ✅ |
| GRPCRouteRule - Section Name                             | Core              |                                                                                                                   ✅ |
| GRPCRouteRule - GRPCRouteMatch - GRPCMethodMatch         | Core              |                                                                                                                   ✅ |
| GRPCRouteRule - GRPCRouteMatch - GRPCHeaderMatch         | Core              |                                                                                                                   ✅ |
| GRPCRouteRule - GRPCRouteFilter - Type                   | Core              |                                                                                                 ❌-- Partial support |
| GRPCRouteRule - GRPCRouteFilter - RequestHeaderModifier  | Core              | ❌-- [Limited Support](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/header-modification.html) |
| GRPCRouteRule - GRPCRouteFilter - ResponseHeaderModifier | Core              |                                                                                                                   ❌ |
| GRPCRouteRule - GRPCRouteFilter - RequestMirror          | Core              |                                                                                                                   ❌ |
| GRPCRouteRule - GRPCRouteFilter - ExtensionRef           | Core              |                       ✅-- Use to attach [ListenerRuleConfigurations](customization.md#customizing-l7-routing-rules) |
| GRPCRouteRule - SessionPersistence                       | Extended          |                                                                                  ❌ -- Use ListenerRuleConfiguration |

##### HTTPRoute

| Field                                                    | Conformance Level |                                                                                                 ALB Gateway Support |
|:---------------------------------------------------------|:------------------|--------------------------------------------------------------------------------------------------------------------:|
| ParentRefs                                               | Core              |                                                                                                                   ✅ |
| UseDefaultGateways                                       | Core              |                                                                                                                   ❌ |
| Hostnames                                                | Core              |                                                                                                                   ✅ |
| HTTPRouteRule - Section Name                             | Core              |                                                                                                                   ✅ |
| HTTPRouteRule - HTTPRouteMatch - HTTPPathMatch           | Core              |                                                                                                                   ✅ |
| HTTPRouteRule - HTTPRouteMatch - HTTPHeaderMatch         | Core              |                                                                                                                   ✅ |
| HTTPRouteRule - HTTPRouteMatch - HTTPQueryParamMatch     | Core              |                                                                                                                   ✅ |
| HTTPRouteRule - HTTPRouteMatch - HTTPMethod              | Core              |                                                                                                                   ✅ |
| HTTPRouteRule - HTTPRouteFilter - Type                   | Core              |                                                                                                ❌ -- Partial support |
| HTTPRouteRule - HTTPRouteFilter - RequestHeaderModifier  | Core              | ❌-- [Limited Support](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/header-modification.html) |
| HTTPRouteRule - HTTPRouteFilter - ResponseHeaderModifier | Core              |                                                                                                                   ❌ |
| HTTPRouteRule - HTTPRouteFilter - RequestMirror          | Extended          |                                                                                                                   ❌ |
| HTTPRouteRule - HTTPRouteFilter - RequestRedirect        | Core              |    ✅ -- See [ReplacePrefixMatch Limitation](#requestredirect-path-modification-replaceprefixmatch-limitation) below |
| HTTPRouteRule - HTTPRouteFilter - UrlRewrite             | Extended          |                                                                                                                   ✅ |
| HTTPRouteRule - HTTPRouteFilter - CORS                   | Extended          |                                                                                                                   ❌ |
| HTTPRouteRule - HTTPRouteFilter - ExternalAuth           | Extended          |                                ❌ -- Use [ListenerRuleConfigurations](customization.md#customizing-l7-routing-rules) |
| HTTPRouteRule - HTTPRouteFilter - ExtensionRef           | Core              |                      ✅ -- Use to attach [ListenerRuleConfigurations](customization.md#customizing-l7-routing-rules) |
| HTTPRouteRule - HTTPBackendRef                           | Core              |                                                                                                                   ✅ |
| HTTPRouteRule - HTTPRouteTimeouts                        | Extended          |                                                                                                                   ❌ |
| HTTPRouteRule - HTTPRouteRetry                           | Extended          |                                                                                                                   ❌ |
| HTTPRouteRule - SessionPersistence                       | Extended          |                                                                                  ❌ -- Use ListenerRuleConfiguration |

##### Backend TLS Policy

Backend TLS is not supported by AWS ALB Gateway. For more information on how AWS ALB communicates with targets using encryption, 
please see the [AWS documentation](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html#target-group-routing-configuration).

##### RequestRedirect Path Modification ReplacePrefixMatch Limitation

The AWS Load Balancer Controller supports HTTPRoute RequestRedirect filters with both `ReplaceFullPath` and `ReplacePrefixMatch` path modification types.

**ReplacePrefixMatch Behavior:**

We support `ReplacePrefixMatch` with limitations:

1. **With scheme/port/hostname changes** - Works as expected:
   ```yaml
   filters:
   - type: RequestRedirect
     requestRedirect:
       scheme: HTTPS  # or port/hostname
       path:
         type: ReplacePrefixMatch
         replacePrefixMatch: /new-prefix
   ```
   - Request: `/old-prefix/path/to/resource`
   - Redirects to: `/new-prefix/path/to/resource` ✅ (suffix preserved)

2. **Without other component changes** - AWS ALB will reject with redirect loop error:
   ```yaml
   filters:
   - type: RequestRedirect
     requestRedirect:
       path:
         type: ReplacePrefixMatch
         replacePrefixMatch: /new-prefix
   ```
   - This configuration will be rejected by the API with "InvalidLoadBalancerAction: The redirect configuration is not valid because it creates a loop." ❌

**Recommendations:**

- For path-only redirects, use `ReplaceFullPath` instead
- To use `ReplacePrefixMatch`, you must also modify `scheme`, `port`, or `hostname`

**Important**: If one HTTPRoute rule has an invalid redirect configuration (e.g., path-only redirect with `ReplacePrefixMatch` that cause redirect loop), the controller will fail to create that listener rule and stop processing subsequent rules in the same HTTPRoute. This means valid rules with lower precedence (shorter paths, later in the route) will not be created. 

#### Examples


##### Modifying Request Headers

AWS ALB only allows [specific](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/header-modification.html) request headers to be modified.

** Request header modification must be done using the LoadBalancerConfiguration, using [Listener Attributes](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/enable-header-modification.html) **

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: example-ns
spec:
  listenerConfigurations:
    - protocolPort: HTTPS:443
      defaultCertificate: my-cert
      listenerAttributes:
        - key: routing.http.response.access_control_allow_origin.header_value
          value: example.com
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
      name: test-gw-lbconfig-1
      group: gateway.k8s.aws
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
      allowedRoutes:
        namespaces:
          from: Same
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-http-app-route
  namespace: example-ns
spec:
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: my-http-gateway
      sectionName: https
  rules:
    - backendRefs:
        - group: ""
          kind: Service
          name: echoserver
          port: 80
          weight: 1
      matches:
        - path:
            type: PathPrefix
            value: /
```

##### HTTP Header Matching

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-http-app-route
  namespace: example-ns
spec:
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: my-http-gateway
      sectionName: https
  rules:
    - backendRefs:
        - group: ""
          kind: Service
          name: echoserver
          port: 80
          weight: 1
      matches:
        - path:
            type: PathPrefix
            value: /
        - headers:
            - name: "oneHeaderSpecial"
              type: Exact
              value: "bar\\,bat"
            - name: "multiHeader"
              type: Exact
              value: "value1,value2"
            - name: "oneHeader"
              type: Exact
              value: "cat"
```

In this example will *only* be forwarded to the echoserver backend when the HTTP Request has these headers:
`oneHeaderSpecial=bar,bat`
AND
`multiHeader=value1` OR `multiHeader=value2`
AND
`oneHeader=cat`



##### Source IP Condition

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: custom-rule-config-source-ip
  namespace: example-ns
spec:
  conditions:
    - field: source-ip
      sourceIPConfig:
        values:
          - 10.0.0.0/5
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-http-app-route
  namespace: example-ns
spec:
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: my-http-gateway
      sectionName: https
  rules:
    - backendRefs:
        - group: ""
          kind: Service
          name: echoserver
          port: 80
          weight: 1
      filters:
        - extensionRef:
            group: gateway.k8s.aws
            kind: ListenerRuleConfiguration
            name: custom-rule-config-source-ip
          type: ExtensionRef
      matches:
        - path:
            type: PathPrefix
            value: /
```

