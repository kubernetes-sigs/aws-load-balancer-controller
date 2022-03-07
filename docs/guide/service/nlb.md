# Network Load Balancer
AWS Load Balancer Controller supports Network Load Balancer (NLB) with instance or IP targets through Kubernetes service of type `LoadBalancer` with proper annotations.

### Instance mode
Instance target mode supports pods running on AWS EC2 instances. In this mode, AWS NLB sends traffic to the instances and the `kube-proxy` on the individual worker nodes forward it to the pods through one or more worker nodes in the Kubernetes cluster.
### IP mode
IP target mode supports pods running on AWS EC2 instances and AWS Fargate. In this mode, the AWS NLB targets traffic directly to the Kubernetes pods behind the service, eliminating the need for an extra network hop through the worker nodes in the Kubernetes cluster.

## Prerequisites
* AWS LoadBalancer Controller >= v2.2.0
* Kubernetes >= v1.16 for Service type `NodePort`
* Kubernetes >= v1.20 or EKS >= 1.16 or the following patch releases for Service type `LoadBalancer`
    - 1.18.18+ for 1.18
    - 1.19.10+ for 1.19
* Pods have native AWS VPC networking configured, see [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s)

!!!note "secure by default"
    Starting v2.2.0 release, the AWS Load balancer controller provisions an internal NLB by default. To create an internet-facing load balancer, apply the following annotation to your service:

    ```
    service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
    ```

    For backwards compatibility, if this annotation is not specified, existing NLB will continue to use the scheme configured on the AWS resource.

## Configuration
The service resources of type `LoadBalancer` also get reconciled by the kubernetes controller built into the cloudprovider component of the `kube-controller-manager` or the `cloud-controller-manager` aka the `in-tree` controller. The AWS in-tree controller
ignores those services resources that have the `service.beta.kubernetes.io/aws-load-balancer-type` annotation as `external`. The AWS Load balancer controller support for NLB is based on the in-tree cloud controller ignoring the service resources, so it is very important
to apply the following annotation on the service resource during creation:

```yaml
service.beta.kubernetes.io/aws-load-balancer-type: "external"
```
This `external` value to the above annotation causes the in-tree controller to not process the service resource and thus pass it on to the external controller.

!!!warning "annotation modification"
    Do not modify or add the `service.beta.kubernetes.io/aws-load-balancer-type` annotation on an existing service object. If you need to make changes, for example from classic to NLB or NLB managed
    by the in-tree controller to the one managed by the AWS Load balancer controller, delete the kubernetes service first and then create again with the correct annotation. If you modify the annotation after service creation
    you will end up with leaked AWS load balancer resources.

### IP mode
NLB IP mode is determined based on the `service.beta.kubernetes.io/aws-load-balancer-nlb-target-type` annotation. If the annotation value is `ip`, then NLB will be provisioned in IP mode. Here is the manifest snippet:
```yaml
    metadata:
      name: my-service
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "external"
        service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "ip"
```

!!!note "backwards compatibility"
    For backwards compatibility, controller still supports the `nlb-ip` as the type annotation. For example, if you specify

    ```
    service.beta.kubernetes.io/aws-load-balancer-type: nlb-ip
    ```

    the controller will provision NLB in IP mode. With this, the `service.beta.kubernetes.io/aws-load-balancer-nlb-target-type` annotation gets ignored.

### Instance mode
Similar to the IP mode, the instance mode is based on the annotation `service.beta.kubernetes.io/aws-load-balancer-nlb-target-type` value `instance`. Here is a sample manifest snippet:
!!!warning "NodePort allocation"
    k8s version 1.22 and later support disabling NodePort allocation by setting the service field `spec.allocateLoadBalancerNodePorts` to `false`. If the NodePort is not allocated for a service port, the controller will fail to reconcile instance mode NLB.

```yaml
    metadata:
      name: my-service
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "external"
        service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "instance"
```

## Protocols
Controller supports both TCP and UDP protocols. Controller also configures TLS termination on NLB if you configure service with certificate annotation. 

In case of TCP, NLB with IP targets does not pass the client source IP address unless specifically configured via target group attributes. Your application pods might not see the actual client IP address even if NLB passes it along, for example instance mode with `externalTrafficPolicy` set to `Cluster`.
In such cases, you can configure [NLB proxy protocl v2](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#proxy-protocol) via [annotation](https://kubernetes.io/docs/concepts/services-networking/service/#proxy-protocol-support-on-aws) if you need visibility into
the client source IP address on your application pods.

To enable proxy protocol v2, apply the following annotation to your service:
```yaml
service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: "*"
```
!!!note ""
    If you enable proxy protocol v2, NLB health check with HTTP/HTTPS works only if the health check port supports proxy protocol v2. Due to this behavior, you should not configure proxy protocol v2 with NLB instance mode and `externalTrafficPolicy` set to `Local`.

## Subnet tagging requirements
See [Subnet Discovery](../../deploy/subnet_discovery.md) for details on configuring ELB for public or private placement.


## Security group
NLB does not currently support managed security groups. For ingress access, the controller adds inbound rules to the node security group for the instance mode, or the ENI security group for the IP mode. In case of multiple
security groups, the controller expects only one security group tagged with the cluster name as follows:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/cluster/${cluster-name}` | `owned` or `shared`   |

`${cluster-name}` is the name of the kubernetes cluster

## Load Balancer Class
The AWS Load Balancer Controller supports `LoadBalancerClass` starting v2.4.0 release on k8s 1.22 or later clusters. The LoadBalancerClass provides a cloudprovider agnostic way of offloading the load balancer reconciliation to an external controller. This controller uses the `service.k8s.aws/nlb` as the default class,
you can configure it to a different value via the controller flag `--load-balancer-class`.

When you specify the `spec.loadBalancerClass` on a service of type `LoadBalancer` during service creation, this controller creates an internal NLB with instance targets by default. If the LoadBalancerClass is not the configured for this controller, this controller ignores the service resource completely regardless of the annotation
`service.beta.kubernetes.io/aws-load-balancer-type`. If you modify the service, with `spec.loadBalancerClass`, type from `LoadBalancer` to anything else, the controller will cleanup the NLB.
