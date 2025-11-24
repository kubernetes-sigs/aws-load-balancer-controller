# Gateway API for Layer 4 (NLB) Implementation

This section details the **AWS Load Balancer Controller's (LBC)** architecture and operational flow when processing Gateway API resources for Layer 4 traffic utilizing AWS NLB.

### Gateway API Resources and Controller Architecture

The LBC implements Gateway API support through a bifurcated architecture, employing distinct controller instances for **Layer 4 (L4)** and **Layer 7 (L7)** routing. This design allows for specialized and optimized reconciliation aligned with the underlying AWS Load Balancer capabilities.

The LBC instances dedicated to L4 routing monitor the following Gateway API resources:

* **`GatewayClass`**: For L4 routing, the LBC specifically manages `GatewayClass` resources with the `controllerName` set to `gateway.k8s.aws/nlb`.
* **`Gateway`**: For every gateway which references a `GatewayClass` with the `controllerName` set to `gateway.k8s.aws/nlb`, The LBC provisions an AWS NLB.
* **`TLSRoute`**: Defines TLS-specific routing rules, enabling secure Layer 4 communication. These routes are satisfied by an **AWS NLB**.
* **`TCPRoute`**: Defines TCP-specific routing rules, facilitating direct TCP traffic management. These routes are satisfied by an **AWS NLB**.
* **`UDPRoute`**: Defines UDP-specific routing rules, facilitating UDP traffic management. These routes are satisfied by an **AWS NLB**.
* **`ReferenceGrant`**: Defines cross-namespace access. For more information [see](https://gateway-api.sigs.k8s.io/api-types/referencegrant/)
* **`LoadBalancerConfiguration` (LBC CRD)**: A Custom Resource Definition utilized for fine-grained customization of the provisioned NLB. This CRD can be attached to a `Gateway` or its `GatewayClass`. For more info, please refer [How customization works](customization.md#customizing-the-gateway-load-balancer-using-loadbalancerconfiguration-crd)
* **`TargetGroupConfiguration` (LBC CRD)**: A Custom Resource Definition used for service-specific customizations of AWS Target Groups. This CRD is associated with a Kubernetes `Service`. For more info, please refer [How customization works](customization.md#customizing-services-target-groups-using-targetgroupconfiguration-crd)

### The Reconciliation Loop

The LBC operates on a continuous **reconciliation loop** within your cluster to maintain the desired state of AWS Load Balancer resources:

1.  **Event Watching:** The L4-specific controller instance constantly monitors the Kubernetes API for changes to the resources mentioned above to NLB provisioning.
2.  **Queueing:** Upon detecting any modification, creation, or deletion of these resources, the respective object is added to an internal processing queue.
3.  **Processing:**
    * The controller retrieves the resource from the queue.
    * It validates the resource's configuration and determines if it falls under its management (e.g., by checking the `GatewayClass`'s `controllerName`). If it does, it enqueues a relevant gateway for processing
    * The Kubernetes Gateway API definition is then translated into an equivalent desired state within AWS (e.g., specific NLB, Listeners, Target Groups etc).
    * This desired state is meticulously compared against the actual state of AWS resources.
    * Necessary AWS API calls are executed to reconcile any identified discrepancies, ensuring the cloud infrastructure matches the Kubernetes declaration.
4.  **Status Updates:** Following reconciliation, the LBC updates the `status` field of the Gateway API resources in Kubernetes. This provides real-time feedback on the provisioned AWS resources, such as the NLB's DNS name and ARN or if the gateways are accepted or not.

### Step-by-Step L4 Gateway API Resource Implementation with an Example

This section illustrates the step-by-step process of configuration of L4 Gateway API resources, demonstrating how the LBC provisions and configures an NLB.

Consider a scenario where an application requires direct TCP traffic routing:

```yaml
# nlb-gatewayclass.yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: GatewayClass
metadata:
  name: aws-nlb-gateway-class
spec:
  controllerName: gateway.k8s.aws/nlb
---
# my-nlb-gateway.yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: my-tcp-gateway
  namespace: example-ns
spec:
  gatewayClassName: aws-nlb-gateway-class
  listeners:
  - name: tcp-app
    protocol: TCP
    port: 8080
    allowedRoutes:
      namespaces:
        from: Same
---
# my-tcproute.yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
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

* **API Event Detection:** The LBC's L4 controller continuously monitors the Kubernetes API server. Upon detecting the `aws-nlb-gateway-class` (with `controllerName: gateway.k8s.aws/nlb`), the `my-tcp-gateway` (referencing this `GatewayClass`), and `my-tcp-app-route` (referencing `my-tcp-gateway`'s `tcp-app` listener) resources, it recognizes its responsibility to manage these objects and initiates the provisioning of AWS resources.
* **NLB Provisioning:** An **AWS Network Load Balancer (NLB)** is provisioned in AWS for the `my-tcp-gateway` resource with default settings. At this stage, the NLB is active but does not yet have any configured listeners. As soon as the NLB becomes active, the status of the gateway is updated.
* **L4 Listener Materialization:** The controller processes the `my-tcp-app-route` resource. Given that the `TCPRoute` validly references the `my-tcp-gateway` and its `tcp-app` listener, an **NLB Listener** is materialized on the provisioned NLB. This listener will be configured for `TCP` protocol on `port 8080`, as specified in the `Gateway`'s listener definition. A default forward action is subsequently configured on the NLB Listener, directing all incoming traffic on `port 8080` to the newly created Target Group for service `my-tcp-service` in `backendRefs` section of `my-tcp-app-route`.
* **Target Group Creation:** An **AWS Target Group** is created for the Kubernetes Service `my-tcp-service` with default configuration. The cluster nodes are then registered as targets within this new Target Group.

### L4 Gateway API Limitations for NLBs
The LBC implementation of the Gateway API for L4 routes, which provisions NLB, introduces specific constraints to align with NLB capabilities. These limitations are enforced during the reconciliation process and are critical for successful L4 traffic management.

#### Single Route Per L4 Gateway Listener:

**Limitation**: Each L4 Gateway Listener (configured via a Gateway resource for TCP, UDP, or TLS protocols) is designed to handle traffic for precisely one L4 Route resource (TCPRoute, UDPRoute, or TLSRoute). The controller does not support scenarios where multiple Route resources attempt to attach to the same L4 Gateway Listener and will throw a validation error during reconcile.

**Reasoning**: This constraint simplifies L4 listener rule management on NLBs, which primarily offer a default action for a given port/protocol.

#### Single Backend Reference Per L4 Route:

**Limitation**: Each L4 Route resource (TCPRoute, UDPRoute, or TLSRoute) must specify exactly one backend reference (backendRef). The controller explicitly disallows routes with zero or more than one backendRef throwing a validation error during reconcile

**Reasoning**: Unlike ALBs which support weighted target groups for traffic splitting across multiple backends, NLBs primarily forward traffic to a single target group for a given listener's default action. This aligns the LBC's L4 route translation with NLB's inherent capabilities, where weighted target groups are not a native feature for directly splitting traffic across multiple services on a single listener.

These validations ensure that the Kubernetes Gateway API definitions for L4 traffic can be correctly and unambiguously translated into the underlying AWS NLB constructs. Adhering to these limitations is essential for stable and predictable L4 load balancing behavior with the AWS Load Balancer Controller.