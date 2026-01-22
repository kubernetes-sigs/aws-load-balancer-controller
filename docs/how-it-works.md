# How AWS Load Balancer controller works

## Design

The following diagram details the AWS components this controller creates. It also demonstrates the route ingress traffic takes from the ALB to the Kubernetes cluster.

![controller-design](assets/images/controller-design.png)

!!!warning "Note"

    The controller manages the configurations of the resources it creates, and we do not recommend out-of-band modifications to these resources because the controller may revert the manual changes during reconciliation. We recommend to use configuration options provided as best practice, such as ingress and service annotations, controller command line flags, IngressClassParams, and Gateway API resources.

### Ingress Creation

This section describes each step (circle) above. This example demonstrates satisfying 1 ingress resource.

**[1]**: The controller watches for [ingress
events](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-controllers) from the API server. When it
finds ingress resources that satisfy its requirements, it begins the creation of AWS resources.

**[2]**: An
[ALB](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/introduction.html) (ELBv2) is created in AWS for the new ingress resource. This ALB can be internet-facing or internal. You can also specify the subnets it's created in
using annotations.

**[3]**: [Target Groups](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html) are created in AWS for each unique Kubernetes service described in the ingress resource.

**[4]**: [Listeners](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html) are created for every port detailed in your ingress resource annotations. When no port is specified, sensible defaults (`80` or `443`) are used. Certificates may also be attached via annotations.

**[5]**: [Rules](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-update-rules.html) are created for each path specified in your ingress resource. This ensures traffic to a specific path is routed to the correct Kubernetes Service.

Along with the above, the controller also...

- deletes AWS components when ingress resources are removed from k8s.
- modifies AWS components when ingress resources change in k8s.
- assembles a list of existing ingress-related AWS components on start-up, allowing you to
  recover if the controller were to be restarted.

### Ingress Traffic
AWS Load Balancer controller supports two traffic modes:

- Instance mode
- IP mode

By default, `Instance mode` is used, users can explicitly select the mode via `alb.ingress.kubernetes.io/target-type` annotation.
#### Instance mode
Ingress traffic starts at the ALB and reaches the Kubernetes nodes through each service's NodePort. This means that services referenced from ingress resources must be exposed by `type:NodePort` in order to be reached by the ALB.
#### IP mode
Ingress traffic starts at the ALB and reaches the Kubernetes pods directly. CNIs must support directly accessible POD ip via [secondary IP addresses on ENI](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-eni.html).

## Gateway API

In addition to Ingress and Service resources, the AWS Load Balancer Controller also supports the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/). Gateway API is a more expressive, extensible, and role-oriented API for managing traffic routing in Kubernetes.

The controller satisfies Gateway API resources as follows:

- **L7 Routes (HTTPRoute, GRPCRoute)**: Provisioned using [Application Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/introduction.html)
- **L4 Routes (TCPRoute, UDPRoute, TLSRoute)**: Provisioned using [Network Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/introduction.html)

For more information on Gateway API support, including prerequisites, configuration, and examples, see the [Gateway API Guide](guide/gateway/gateway.md).

### How Gateway API Works

The AWS Load Balancer Controller runs a continuous reconciliation loop to align the desired state expressed through Gateway API objects with the actual state of AWS Load Balancer infrastructure. The controller runs dedicated controller instances for L4 routing (NLB) and L7 routing (ALB), each following a similar workflow.

The following diagram illustrates the Gateway API reconciliation process:

![gateway-reconcile](assets/images/gateway-reconcile.png)

At a high level, the reconciliation loop works as follows:

**[1] API Monitoring**: The controller continuously monitors the Kubernetes API for Gateway API resources being created, modified, or deleted.

**[2] Queueing**: Identified resources are added to an internal queue for processing.

**[3] Processing**: For each item in the queue:

- The associated GatewayClass is verified to determine if it is or should be a managed resource.
- If managed, the Gateway API definition is mapped to AWS resources such as NLB/ALB, Listeners, Listener Rules, Target Groups, and Addons.
- These mapped resources are compared with the actual state in AWS. For any resource that does not match the desired state, the controller executes the necessary AWS API calls to reconcile the differences.

**[4] Status Updates**: After reconciliation, the controller updates the status field of the corresponding Gateway resource. This provides real-time feedback on provisioned AWS resources, such as the load balancer's DNS name and ARN, and whether the Gateway is accepted and programmed.

