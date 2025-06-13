# TargetGroupConfiguration

### TargetReference

`targetReference`

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: TargetGroupConfiguration
metadata:
  name: example-tg-config
  namespace: example-ns
spec:
  targetReference:
    name: my-service
```

Defines the Kubernetes object to attach the Target Group settings to.

- **group**: The group of the referent. For example, "gateway.networking.k8s.io". When unspecified or empty string, core API group is inferred.
- **kind**: The Kubernetes resource kind of the referent. For example "Service". Defaults to "Service" when not specified.
- **name**: The name of the referent.

**Default** No default, required field

### DefaultConfiguration

`defaultConfiguration`

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: TargetGroupConfiguration
metadata:
  name: example-tg-config
  namespace: example-ns
spec:
  targetReference:
    name: my-service
  defaultConfiguration:
    targetGroupName: my-target-group
    targetType: ip
```

Defines fallback configuration applied to all routes, unless overridden by route-specific configurations.

See [TargetGroupProps](#targetgroupprops) for more details.

**Default** Empty object

### RouteConfigurations

`routeConfigurations`

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: TargetGroupConfiguration
metadata:
  name: route-specific-config
  namespace: example-ns
spec:
  targetReference:
    name: my-service
  defaultConfiguration:
    targetType: ip
  routeConfigurations:
    - routeIdentifier:
        kind: HTTPRoute
        namespace: example-ns-1
        name: api-route
      targetGroupProps:
        healthCheckConfig:
          healthCheckPath: /api/health
          healthCheckProtocol: HTTP
    - routeIdentifier:
        kind: HTTPRoute
        namespace: example-ns-2
        name: admin-route
      targetGroupProps:
        healthCheckConfig:
          healthCheckPath: /admin/health
          healthCheckInterval: 10
```
#### Route Matching Logic

When applying route-specific configurations, the controller uses a specificity-based matching algorithm to find the best configuration for each route:

1. Most specific match: Kind + Namespace + Name (exact match)
2. Namespace-scoped match: Kind + Namespace (matches all routes of that Kind in the specified Namespace)
3. Kind-only match: Kind (matches all routes of that Kind across all Namespaces)

The matching is strict - if a namespace or name is specified in a routeIdentifier but doesn't exactly match the route, that configuration will not be applied. For example, a routeIdentifier with `{kind: "HTTPRoute", namespace: "test"}` will not match an HTTPRoute in the "default" namespace.

**Default** Empty list

## RouteConfiguration

### RouteIdentifier

`routeIdentifier`

```yaml
routeIdentifier:
  kind: HTTPRoute
  namespace: example-ns
  name: my-route
```

The complete set of route attributes that identify a route.

- **kind**: The Kubernetes resource kind of the route.
- **namespace**: The namespace of the route.
- **name**: The name of the route.

**Default** No default, required field

### TargetGroupProps

`targetGroupProps`

The target group specific properties. See [TargetGroupProps](#targetgroupprops).

**Default** No default, required field

## TargetGroupProps

### TargetGroupName

`targetGroupName`

```yaml
targetGroupName: my-target-group
```

Specifies the name to assign to the Target Group. If not defined, then one is generated.

**Default** Auto-generate name

### IPAddressType

`ipAddressType`

```yaml
ipAddressType: ipv4
```

Specifies whether the target group is of type IPv4 or IPv6. If unspecified, it will be automatically inferred.

Options:
- ipv4
- ipv6

**Default** Auto-inferred

### HealthCheckConfig

`healthCheckConfig`

```yaml
healthCheckConfig:
  healthyThresholdCount: 5
  healthCheckInterval: 30
  healthCheckPath: /healthz
  healthCheckPort: "8080"
  healthCheckProtocol: HTTP
  healthCheckTimeout: 5
  unhealthyThresholdCount: 2
  matcher:
    httpCode: "200"
```

The Health Check configuration for this backend.

See [HealthCheckConfiguration](#healthcheckconfiguration) for more details.

**Default** Default health check configuration from AWS

### NodeSelector

`nodeSelector`

```yaml
nodeSelector:
  matchLabels:
    role: backend
```

Node selector for instance type target groups to only register certain nodes.

**Default** No selector

### TargetGroupAttributes

`targetGroupAttributes`

```yaml
targetGroupAttributes:
  - key: deregistration_delay.timeout_seconds
    value: "30"
  - key: stickiness.enabled
    value: "true"
```

Defines the attribute of target group.

**Default** Empty list

### TargetType

`targetType`

```yaml
targetType: ip
```

The TargetType of TargetGroup.

Options:
- instance: Nodes with nodePort for your service will be registered as targets
- ip: Pods with containerPort for your service will be registered as targets

**Default** Auto-inferred as "instance"

### Protocol

`protocol`

```yaml
protocol: HTTP
```

The protocol for the target group. If unspecified, it will be automatically inferred.

Options:
- HTTP
- HTTPS
- TCP
- TLS
- UDP
- TCP_UDP

**Default** Auto-inferred

### ProtocolVersion

`protocolVersion`

```yaml
protocolVersion: HTTP2
```

The protocol version. Only applicable for HTTP/HTTPS protocol.

Options:
- HTTP1
- HTTP2
- GRPC

**Default** No default

### EnableMultiCluster

`enableMultiCluster`

```yaml
enableMultiCluster: true
```

Allows for multiple Clusters / Services to use the generated TargetGroup ARN.

**Default** false

### Tags

`tags`

```yaml
tags:
  Environment: Production
  Project: MyApp
```

The Tags to add on the target group.

**Default** No tags

## HealthCheckConfiguration

### HealthyThresholdCount

`healthyThresholdCount`

```yaml
healthyThresholdCount: 5
```

The number of consecutive health checks successes required before considering an unhealthy target healthy.

**Default** AWS default value

### HealthCheckInterval

`healthCheckInterval`

```yaml
healthCheckInterval: 30
```

The approximate amount of time, in seconds, between health checks of an individual target.

**Default** AWS default value

### HealthCheckPath

`healthCheckPath`

```yaml
healthCheckPath: /healthz
```

The destination for health checks on the targets.

**Default** AWS default path

### HealthCheckPort

`healthCheckPort`

```yaml
healthCheckPort: "8080"
```

The port the load balancer uses when performing health checks on targets. The default is to use the port on which each target receives traffic from the load balancer.

**Default** Use target port

### HealthCheckProtocol

`healthCheckProtocol`

```yaml
healthCheckProtocol: HTTP
```

The protocol to use to connect with the target. The GENEVE, TLS, UDP, and TCP_UDP protocols are not supported for health checks.

Options:
- HTTP
- HTTPS
- TCP

**Default** AWS default protocol

### HealthCheckTimeout

`healthCheckTimeout`

```yaml
healthCheckTimeout: 5
```

The amount of time, in seconds, during which no response means a failed health check.

**Default** AWS default timeout

### UnhealthyThresholdCount

`unhealthyThresholdCount`

```yaml
unhealthyThresholdCount: 2
```

The number of consecutive health check failures required before considering the target unhealthy.

**Default** AWS default count

### Matcher

`matcher`

```yaml
matcher:
  httpCode: "200"
```

```yaml
matcher:
  grpcCode: "0"
```

The HTTP or gRPC codes to use when checking for a successful response from a target.

Note: Only one of httpCode or grpcCode should be set.

**Default** AWS default matcher codes

## HealthCheckMatcher

### HTTPCode

`httpCode`

```yaml
httpCode: "200"
```

The HTTP codes to consider a successful health check.

**Default** No default if specified

### GRPCCode

`grpcCode`

```yaml
grpcCode: "0"
```

The gRPC codes to consider a successful health check.

**Default** No default if specified