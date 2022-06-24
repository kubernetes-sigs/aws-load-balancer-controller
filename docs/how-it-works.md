# How AWS Load Balancer controller works

## Design

The following diagram details the AWS components this controller creates. It also demonstrates the route ingress traffic takes from the ALB to the Kubernetes cluster.

![controller-design](assets/images/controller-design.png)

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

## Lifecycle of AWS Load Balancers 

!!! warning 
    The controller, generally, does not update load balancers after creation. 


The controller updates the targets of load balancers in response to cluster workload changes. However, the controller may not update the configuration of load balancers in response to annotations added to cluster resources after the load balancer is created. 
    
For example, if you add the annotation `aws-load-balancer-ssl-cert` to a service after the load balancer is created, the controller will *not* update the configuration of the AWS load balancer. However, if you delete the AWS load balancer, the controller will *recreate* the load balancer with the desired configuration.

Recommendations: 
1. Assume AWS load balancers may need to be replaced.
1. Do not manually update the configuration of load balancers provisioned by the controller. This will cause the in-cluster resource definitions and the AWS configuration to get out of sync. 
1. If durability of a specific load balancer resource is important, consider using IAM permissions to block the controller from creating or deleting new load balancers. Use the controller only to update the targets of self-managed load balancers.
1. Use the [ExternalDNS Controller](https://github.com/kubernetes-sigs/external-dns) to automatically update DNS records instead of relying on a specific load balancer IP address or hostname. 



