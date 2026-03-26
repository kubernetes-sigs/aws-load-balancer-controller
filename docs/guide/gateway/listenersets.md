## ListenerSets

A `ListenerSet` is a Gateway API resource that allows you to attach additional listeners to an existing Gateway without modifying the Gateway resource itself. This is defined in [GEP-1713](https://gateway-api.sigs.k8s.io/geps/gep-1713/).
`ListenerSet` was added in Gateway API verson v1.5.0 and AWS Load Balancer Controller v3.2.0.

Both ALB (`gateway.k8s.aws/alb`) and NLB (`gateway.k8s.aws/nlb`) Gateway controllers support ListenerSets.

### When to use ListenerSets

ListenerSets are useful when:

- **Delegating listener management**: Application teams can manage their own listeners (e.g., with unique hostnames and TLS certificates) without requiring access to the central Gateway resource.
- **Scaling beyond Gateway listener limits**: The Gateway API limits a Gateway to 64 listeners. ListenerSets allow you to exceed this by distributing listeners across multiple resources.
- **Cross-namespace listener attachment**: Infrastructure teams can own the Gateway while allowing workload namespaces to attach their own listeners via namespace-scoped ListenerSets.

### How to use ListenerSets

#### 1. Enable ListenerSet attachment on the Gateway

By default, Gateways do not accept ListenerSets. You must configure `allowedListeners` on the Gateway to opt in.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  namespace: infra
spec:
  gatewayClassName: aws-alb-gateway-class
  allowedListeners:
    namespaces:
      from: Same # Options: Same, All, Selector, None
  listeners:
    - name: default-http
      protocol: HTTP
      port: 80
```

The `from` field controls which namespaces can attach ListenerSets:

| Value      | Behavior                                               |
| :--------- | :----------------------------------------------------- |
| `None`     | No ListenerSets allowed (default)                      |
| `Same`     | Only ListenerSets in the same namespace as the Gateway |
| `All`      | ListenerSets from any namespace                        |
| `Selector` | ListenerSets from namespaces matching a label selector |

When using `Selector`, provide a label selector:

```yaml
allowedListeners:
  namespaces:
    from: Selector
    selector:
      matchLabels:
        team: frontend
```

#### 2. Create a ListenerSet

A ListenerSet references a parent Gateway and defines one or more listeners:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: ListenerSet
metadata:
  name: app-listeners
  namespace: infra
spec:
  parentRef:
    name: my-gateway
    kind: Gateway
    group: gateway.networking.k8s.io
  listeners:
    - name: app-http
      port: 8080
      protocol: HTTP
    - name: app-https
      port: 443
      protocol: HTTPS
      hostname: app.example.com
```

#### 3. Attach routes to ListenerSet listeners

Routes can reference a ListenerSet directly as a `parentRef`. Use `sectionName` to target a specific listener within the ListenerSet:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: app-route
  namespace: infra
spec:
  parentRefs:
    - name: app-listeners
      kind: ListenerSet
      group: gateway.networking.k8s.io
      sectionName: app-http
  rules:
    - backendRefs:
        - name: my-service
          port: 80
```

To attach a route to both a Gateway listener and a ListenerSet listener, specify multiple `parentRefs`:

```yaml
parentRefs:
  - name: my-gateway
    kind: Gateway
    sectionName: default-http
  - name: app-listeners
    kind: ListenerSet
    group: gateway.networking.k8s.io
    sectionName: app-http
```

!!! note
A route targeting a Gateway with no `sectionName` attaches only to the Gateway's own `spec.listeners`, not to any ListenerSet listeners. This is by design in the Gateway API specification.

!!! note
Listeners defined in a ListenerSet use the ListenerSet's namespace for route attachment checks. For example, when a listener's `allowedRoutes` is set to `from: Same`, routes must be in the same namespace as the ListenerSet, not the parent Gateway.

#### Listener precedence and conflict resolution

When listeners from the Gateway and attached ListenerSets are merged, the following precedence applies:

1. Parent Gateway listeners (highest priority)
2. ListenerSet listeners ordered by creation time (oldest first)
3. ListenerSet listeners ordered alphabetically by `{namespace}/{name}`

If a ListenerSet listener conflicts with a higher-priority listener (e.g., same port and hostname), the conflicting listener is marked with a `Conflicted` condition and is not programmed on the load balancer. The higher-priority listener remains unaffected.

Listener names do not need to be unique across the Gateway and its ListenerSets. They only need to be unique within a single resource.

### Observability

#### Gateway status

The Gateway's `status.attachedListenerSets` field reports the number of ListenerSets that have been successfully accepted:

```yaml
status:
  attachedListenerSets: 2
  addresses:
    - type: Hostname
      value: my-alb-1234567890.us-west-2.elb.amazonaws.com
```

#### ListenerSet status

Each ListenerSet has top-level `Accepted` and `Programmed` conditions, plus per-listener status:

```yaml
status:
  conditions:
    - type: Accepted
      status: "True"
      reason: Accepted
    - type: Programmed
      status: "True"
      reason: Programmed
  listeners:
    - name: app-http
      attachedRoutes: 1
      conditions:
        - type: Accepted
          status: "True"
          reason: Accepted
        - type: Programmed
          status: "True"
          reason: Programmed
        - type: Conflicted
          status: "False"
          reason: NoConflicts
        - type: ResolvedRefs
          status: "True"
          reason: ResolvedRefs
```

Common reasons for a ListenerSet not being accepted:

| Condition  | Reason              | Meaning                                                        |
| :--------- | :------------------ | :------------------------------------------------------------- |
| Accepted   | `NotAllowed`        | The Gateway does not allow ListenerSets from this namespace    |
| Accepted   | `ListenersNotValid` | One or more listeners have validation errors                   |
| Programmed | `Pending`           | The parent Gateway is not yet programmed or no valid listeners |

Individual listener conditions follow the same semantics as Gateway listener conditions (`Accepted`, `Programmed`, `Conflicted`, `ResolvedRefs`).
