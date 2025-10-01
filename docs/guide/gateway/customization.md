## Customizing your ELB resources

The AWS Load Balancer Controller (LBC) provides sensible defaults for provisioning and managing Elastic Load Balancing (ELB) resources in response to Kubernetes Gateway API objects. However, to accommodate diverse use cases and specific operational requirements, the LBC offers extensive, fine-grained customization capabilities through three Custom Resource Definitions (CRDs): [LoadBalancerConfiguration](./spec.md#loadbalancerconfiguration), [TargetGroupConfiguration](./spec.md/#targetgroupconfiguration), and [ListenerRuleConfiguration](./spec.md#listenerruleconfiguration).

![screen showing all LBC Gateway API components](assets/gateway-full.png)

### Customizing the Gateway (Load Balancer) using `LoadBalancerConfiguration` CRD

The `LoadBalancerConfiguration` CRD allows for the detailed customization of the AWS Load Balancer (ALB or NLB) provisioned by the LBC for a given Gateway.

For a comprehensive list of configurable parameters, please refer to the [LoadBalancerConfiguration CRD documentation](./loadbalancerconfig.md).

**Example:** To configure your Gateway to provision an `internet-facing` Load Balancer, define the following `LoadBalancerConfiguration` resource:

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: internet-facing-config
  namespace: example-ns
spec:
  scheme: internet-facing
```

This configuration can then be applied by attaching the `LoadBalancerConfiguration` resource to either a `Gateway` or a `GatewayClass`.

**Attaching to a Gateway:**
When attached directly to a `Gateway` resource, the specified configuration applies specifically to the Load Balancer provisioned for that individual Gateway.

!!! note
    Make sure that the `LoadBalancerConfiguration` is located in the same namespace as the `Gateway`.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: test-gw
  namespace: example-ns
spec:
  gatewayClassName: nlb-gateway
  infrastructure:
    parametersRef:
      group: gateway.k8s.aws
      kind: LoadBalancerConfiguration
      name: internet-facing-config # Must be in the same namespace as the Gateway
  listeners:
  ...
```

**Attaching to a GatewayClass:**
When attached to a `GatewayClass` resource, the configuration becomes a default for all `Gateway` resources that reference this `GatewayClass`.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nlb-gateway
spec:
  controllerName: gateway.k8s.aws/alb
  parametersRef:
    group: gateway.k8s.aws
    kind: LoadBalancerConfiguration
    name: internet-facing-config
    namespace: example-ns
```

#### Conflict Resolution for `LoadBalancerConfiguration`

It is possible for a `LoadBalancerConfiguration` to be attached to both a `Gateway` and its associated `GatewayClass`. In such scenarios, when identical fields are specified in both configurations, the LBC employs a merging algorithm to resolve conflicts. The precedence of values is determined by the `mergingMode` field, which is exclusively read from the `GatewayClass`'s `LoadBalancerConfiguration`. If `mergingMode` is not explicitly set, the `GatewayClass` configuration implicitly takes higher precedence. For more info on `mergingMode`, refer to [the merging mode documentation](./loadbalancerconfig.md#mergingmode).

The following fields exhibit specific merge behaviors:

* **`tags`**: The tag maps from both configurations are combined. In the event of duplicate tag keys, the value from the higher-priority configuration (as determined by `mergingMode`) will be utilized.
* **`loadBalancerAttributes`**: The attribute lists are combined. For duplicate attribute keys, the value from the higher-priority configuration will prevail.
* **`mergeListenerConfig`**: Listener lists are combined. For duplicate `ProtocolPort` keys, the listener configuration from the higher-priority source will be selected.

-----

### Customizing Services (Target Groups) using `TargetGroupConfiguration` CRD

The `TargetGroupConfiguration` CRD enables granular customization of the AWS Target Groups created for Kubernetes Services.

For a comprehensive overview of configurable parameters, please refer to the  [TargetGroupConfiguration CRD documentation](./targetgroupconfig.md).

**Example: Default Target Group Configuration for a Service**

To configure the target groups for a specific service (e.g., `my-service`) to use `IP` mode and custom health checks across all routes referencing it, employ the following configuration:

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: TargetGroupConfiguration
metadata:
  name: custom-tg-config
  namespace: example-namespace
spec:
  targetReference:
    name: my-service
  defaultConfiguration:
    targetType: ip
    healthCheckConfig:
      healthCheckPath: /health
      healthCheckInterval: 30
      healthyThresholdCount: 3
```

Here, `my-service` is referenced within the `targetReference` of `custom-tg-config`. Any target group subsequently created for `my-service` via any route will inherit these default settings. Note that only one `TargetGroupConfiguration` CRD can be declared per service, and it must reside within the same namespace as the service it configures.

**Example: Route-Specific Target Group Configuration**

Alternatively, specific target group settings can be applied based on the individual routes referencing a service. This allows for tailored configurations for different traffic flows.

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: TargetGroupConfiguration
metadata:
  name: route-specific-tg-config
  namespace: example-ns
spec:
  targetReference:
    name: my-service
  defaultConfiguration:
    targetType: ip
  routeConfigurations:
    - routeIdentifier:
        kind: TCPRoute
        namespace: example-ns
        name: api-route
      targetGroupProps:
        healthCheckConfig:
          healthCheckPath: /api/health
          healthCheckProtocol: HTTP
    - routeIdentifier:
        kind: TCPRoute
        namespace: example-ns-2
        name: admin-route
      targetGroupProps:
        healthCheckConfig:
          healthCheckPath: /admin/health
          healthCheckInterval: 10
```

#### How Default and Route-Specific Configurations Merge

When both `defaultConfiguration` and `routeConfigurations` within a `TargetGroupConfiguration` specify the same field, route-specific configurations take precedence. The controller identifies the most relevant route specification from the list of `routeConfigurations` and merges its `targetGroupProps` with the `defaultConfiguration`'s settings. For detailed information on the route matching logic employed, refer to the [Route Matching section](./targetgroupconfig.md#route-matching-logic).

The following fields exhibit specific merge behaviors:

* **`tags`**: The two tag maps are combined. Any duplicate tag keys will result in the value from the higher-priority (route-specific) configuration being used.
* **`targetGroupAttributes`**: The two attribute lists are combined. Any duplicate attribute keys will result in the attribute value from the higher-priority (route-specific) configuration being applied.

#### Customizing L7 Routing Rules

The `ListenerRuleConfiguration` CRD allows representation of features present in AWS ALB,
that are not represented in the standard Gateway API spec.

An exhaustive list is:

- [Cognito Authentication](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html#cognito-requirements)
- [OIDC Authentication](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html#oidc-requirements)
- [Fixed Response](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/rule-action-types.html#fixed-response-actions)
- [Source IP Conditions](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_SourceIpConditionConfig.html#API_SourceIpConditionConfig_Contents)

For a comprehensive overview of the CRD, please refer to the [ListenerRuleConfiguration CRD documentation](./listenerruleconfig.md).

**Example: Adding source IP routing conditions**

This example adds upon the example found [here](./l7gateway.md#step-by-step-l7-gateway-api-resource-implementation-with-an-example). It adds
a routing rule that only allows requests originating from the range 10.0.0.0/5 to be routed to the backend.

```
# source-ip-condition.yaml
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
# updated-http-route.yaml
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
    filters:
      - type: ExtensionRef
        extensionRef:
          group: "gateway.k8s.aws"
          kind: "ListenerRuleConfiguration"
          name: "custom-rule-config-source-ip"
```

To add granular rules, specify the index match:

```
# source-ip-condition.yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: custom-rule-config-source-ip
  namespace: example-ns
spec:
  conditions:
    - field: source-ip
      matchIndexes: [0,1]
      sourceIPConfig:
        values:
          - 10.0.0.0/5
---
# updated-http-route.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: http-app-source-ip
  namespace: example-ns
spec:
  parentRefs:
    - name: my-alb-gateway
      port: 90
  rules:
    - backendRefs:
        - name: echoserver
          port: 80
      filters:
        - type: ExtensionRef
          extensionRef:
            group: "gateway.k8s.aws"
            kind: "ListenerRuleConfiguration"
            name: "custom-rule-config-source-ip"
      matches:
        - path: # Path Pattern
            type: Exact
            value: /pathExactMatch
          queryParams: # Query String
            - name: "user"
              value: "john"
          method: GET # HTTP Request Method
        - path: # Regex path match
            type: RegularExpression
            value: "/firstRule/some?/users"
    - backendRefs:
        - name: echoserver
          port: 80
      filters:
        - type: ExtensionRef
          extensionRef:
            group: "gateway.k8s.aws"
            kind: "ListenerRuleConfiguration"
            name: "custom-rule-config-source-ip-2"
      matches:
        - path: # Path Pattern
            type: Exact
            value: /secondRulePath
          method: POST # HTTP Request Method
        - path: # Regex path match
            type: RegularExpression
            value: "/secondRule/some?/users"
        - path:
            type: "PathPrefix"
            value: "/secondRule"
```