# NLB IP mode
AWS Load Balancer Controller supports Network Load Balancer (NLB) with IP targets for pods running on Amazon EC2 instances and AWS Fargate through Kubernetes service of type `LoadBalancer` with proper annotation. In this mode, the AWS NLB targets traffic directly to the Kubernetes pods behind the service, eliminating the need for an extra network hop through the worker nodes in the Kubernetes cluster.

## Prerequisites
* AWS LoadBalancer Controller >= v2.0.0
* Kubernetes >= v1.15 for Service type NodePort.
* Kubernetes >= v1.20 or EKS >= 1.18 for Service type LoadBalancer.
* Pods have native AWS VPC networking configured, see [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s)

## Configuration
The NLB IP mode is determined based on the annotations added to the service object. For NLB in IP mode, apply the following annotation to the service:
```yaml
    metadata:
      name: my-service
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb-ip"
```

!!!note ""
    Do not modify the service annotation `service.beta.kubernetes.io/aws-load-balancer-type` on an existing service object. If you need to modify the underlying AWS LoadBalancer type, for example from classic to NLB, delete the kubernetes service first and create again with the correct annotation. Failure to do so will result in leaked AWS load balancer resources.

    The default load balancer is internet-facing. To create an internal load balancer, apply the following annotation to your service:

    ```
    service.beta.kubernetes.io/aws-load-balancer-internal: "true"
    ```

## Protocols
Support is available for both TCP and UDP protocols. In case of TCP, NLB in IP mode does not pass the client source IP address to the pods. You can configure [NLB proxy protocol v2](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#proxy-protocol) via [annotation](https://kubernetes.io/docs/concepts/services-networking/service/#proxy-protocol-support-on-aws) if you need the client source IP address.

to enable proxy protocol v2, apply the following annotation to your service:
```yaml
service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: "*"
```

## Security group
NLB does not currently support a managed security group. For ingress access, the controller will resolve the security group for the ENI corresponding tho the endpoint pod. If the ENI has a single security group, it gets used. In case of multiple security groups, the controller expects to find only one security group tagged with the Kubernetes cluster id. Controller will update the ingress rules on the security groups as per the service spec.