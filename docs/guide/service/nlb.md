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
Similar to the IP mode, the instance mode is based on the annotation `service.beta.kubernetes.io/aws-load-balancer-type` value `instance`. Here is a sample manifest snippet:

```yaml
    metadata:
      name: my-service
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "external"
        service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "instance"
```

## Protocols
Support is available for both TCP and UDP protocols. In case of TCP, NLB in IP mode does not pass the client source IP address to the pods. You can configure [NLB proxy protocol v2](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#proxy-protocol) via [annotation](https://kubernetes.io/docs/concepts/services-networking/service/#proxy-protocol-support-on-aws) if you need the client source IP address.

to enable proxy protocol v2, apply the following annotation to your service:
```yaml
service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: "*"
```

## Subnet tagging requirements
See [Subnet Discovery](https://kubernetes-sigs.github.io/aws-load-balancer-controller/guide/controller/subnet_discovery/) for details on configuring ELB for public or private placement.


## Security group
NLB does not currently support a managed security group. For ingress access, the controller will resolve the security group for the ENI corresponding to the endpoint pod for IP mode, and the security group of the worker nodes for instance mode. If there is a single security group attached to the the ENI or the instance, it gets used. In case of multiple security groups, the controller expects to find only one security group tagged with the Kubernetes cluster id. Controller will update the ingress rules on the security groups as per the service spec.
