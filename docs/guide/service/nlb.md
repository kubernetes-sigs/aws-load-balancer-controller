# Network Load Balancer

AWS Load Balancer Controller supports reconciliation for Kubernetes Services resources of type `LoadBalancer` by Network Load Balancer (NLB) with `instance` or `ip` target type.

!!! info "secure by default"
    Since [:octicons-tag-24: v2.2.0](https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/tag/v2.2.0) release, the AWS Load balancer controller provisions an `internal` NLB by default. 
    
    To create an `internet-facing` NLB, following annotation is required on your service:

    ```yaml
    service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
    ```

    !!! tip ""
        For backwards compatibility, if the [`service.beta.kubernetes.io/aws-load-balancer-scheme`](./annotations.md#lb-scheme) annotation is absent, existing NLB's scheme will remain unchanged.

## Prerequisites
* AWS Load Balancer Controller >= v2.2.0
* For Kubernetes Services resources of type `LoadBalancer`:
    * Kubernetes >= v1.20 or
    * Kubernetes >= v1.19.10 for 1.19 or
    * Kubernetes >= v1.18.18 for 1.18 or
    * EKS >= v1.16
* For Kubernetes Services resources of type `NodePort`:
    * Kubernetes >= v1.16
* For `ip` target type:
    * Pods have native AWS VPC networking configured, see [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s)

## Configuration

By default, Kubernetes Service resources of type `LoadBalancer` gets reconciled by the Kubernetes controller built into the CloudProvider component of the kube-controller-manager or the cloud-controller-manager(a.k.a. the in-tree controller). 

In order to let AWS Load Balancer Controller manage the reconciliation for Kubernetes Services resources of type `LoadBalancer`, you need to offload the reconciliation from in-tree controller to AWS Load Balancer Controller explicitly.


=== "With LoadBalancerClass"
    AWS Load Balancer Controller supports `LoadBalancerClass` feature since [:octicons-tag-24: v2.4.0](https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/tag/v2.4.0) release for Kubernetes v1.22+ clusters. 
     
    `LoadBalancerClass` feature provides a CloudProvider agnostic way of offloading the reconciliation for Kubernetes Services resources of type `LoadBalancer` to an external controller.
    
    When you specify the `spec.loadBalancerClass` to be `service.k8s.aws/nlb` on a Kubernetes Service resource of type `LoadBalancer`, the AWS Load Balancer Controller takes charge of reconciliation by provision an NLB.

    !!! warning
        - If you modify a Service resource with matching `spec.loadBalancerClass` by changing its `type` from `LoadBalancer` to anything else, the controller will cleanup provioned NLB for that Service.

        - If the `spec.loadBalancerClass` is set to a loadBalancerClass that is not recognized by this controller, it will ignore the Service resource regardless of the `service.beta.kubernetes.io/aws-load-balancer-type` annotation.

    !!! tip
        - By default, the NLB will use `instance` target-type, you can customize it via the [`service.beta.kubernetes.io/aws-load-balancer-nlb-target-type` annotation](./annotations.md#nlb-target-type)

        - This controller uses `service.k8s.aws/nlb` as the default `LoadBalancerClass`, you can customize it to a different value via the controller flag `--load-balancer-class`.

    !!! example "Example: instance mode"
        ```yaml hl_lines="6 15"
        apiVersion: v1
        kind: Service
        metadata:
          name: echoserver
          annotations:
            service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: instance
        spec:
          selector:
            app: echoserver
          ports:
            - port: 80
              targetPort: 8080
              protocol: TCP
          type: LoadBalancer
          loadBalancerClass: service.k8s.aws/nlb
        ```

    !!! example "Example: ip mode"
        ```yaml hl_lines="6 15"
        apiVersion: v1
        kind: Service
        metadata:
          name: echoserver
          annotations:
            service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: ip
        spec:
          selector:
            app: echoserver
          ports:
            - port: 80
              targetPort: 8080
              protocol: TCP
          type: LoadBalancer
          loadBalancerClass: service.k8s.aws/nlb
        ```

=== "With `service.beta.kubernetes.io/aws-load-balancer-type` annotation"
    The AWS in-tree controller supports an AWS specific way of offloading the reconciliation for Kubernetes Services resources of type `LoadBalancer` to an external controller. 

    When you specify the [`service.beta.kubernetes.io/aws-load-balancer-type` annotation](./annotations.md#lb-type) to be `external` on a Kubernetes Service resource of type `LoadBalancer`, the in-tree controller will ignore the Service resource. In addition, If you specify the [`service.beta.kubernetes.io/aws-load-balancer-nlb-target-type` annotation](./annotations.md#nlb-target-type) on the Service resource, the AWS Load Balancer Controller takes charge of reconciliation by provision an NLB.

    !!! warning
        - It's not recommended to modify or add the `service.beta.kubernetes.io/aws-load-balancer-type` annotation on an existing Service resource. Instead, delete the existing Service resource and recreate a new one if a change is desired.

        - If you modify this annotation on a existing Service resource, you might end up with leaked AWS Load Balancer resources. 

    !!! note "backwards compatibility for `nlb-ip` type"
        For backwards compatibility, both in-tree and AWS Load Balancer controller supports `nlb-ip` as value of `service.beta.kubernetes.io/aws-load-balancer-type` annotation. The controllers treats it as if you specified both annotation below:
        ```
        service.beta.kubernetes.io/aws-load-balancer-type: external
        service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: ip
        ```
    
    !!! example "Example: instance mode"
        ```yaml hl_lines="6 7"
        apiVersion: v1
        kind: Service
        metadata:
          name: echoserver
          annotations:
            service.beta.kubernetes.io/aws-load-balancer-type: external
            service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: instance
        spec:
          selector:
            app: echoserver
          ports:
            - port: 80
              targetPort: 8080
              protocol: TCP
          type: LoadBalancer
        ```

    !!! example "Example: ip mode"
        ```yaml hl_lines="6 7"
        apiVersion: v1
        kind: Service
        metadata:
          name: echoserver
          annotations:
            service.beta.kubernetes.io/aws-load-balancer-type: external
            service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: ip
        spec:
          selector:
            app: echoserver
          ports:
            - port: 80
              targetPort: 8080
              protocol: TCP
          type: LoadBalancer
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
AWS currently does not support attaching security groups to NLB. To allow inbound traffic from NLB, the controller automatically adds inbound rules to the worker node security groups by default.

!!! tip "disable worker node security group rule management"
    You can disable the worker node security group rule management via [annotation](./annotations.md#manage-backend-sg-rules).

### Worker node security groups selection
The controller automatically selects the worker node security groups that will be modified to allow inbound traffic with following rules:

* for `instance` mode, the security group of the each backend worker node's primary ENI will be selected.
* for `ip` mode, the security group of each backend pod's ENI will be selected.

!!! important "multiple security groups on an ENI"

    if there are multiple security groups attached on an ENI, the controller expects only one security group tagged with following tags:

    | Key                                     | Value               |
    | --------------------------------------- | ------------------- |
    | `kubernetes.io/cluster/${cluster-name}` | `owned` or `shared` |

    `${cluster-name}` is the name of the kubernetes cluster

### Worker node security groups rules

=== "when Client IP preservation enabled"

    | Rule                 | Protocol                 | Port(s)                                                 | IpRanges(s)                                         |
    | -------------------- | ------------------------ | ------------------------------------------------------- | --------------------------------------------------- |
    | Client Traffic       | `spec.ports[*].protocol` | `spec.ports[*].port`                                    | [Traffic Source CIDRs](./annotations.md#lb-source-ranges) |
    | Health Check Traffic | TCP                      | [Health Check Ports](./annotations.md#healthcheck-port) | NLB Subnet CIDRs                                        |

=== "when Client IP preservation disabled"

    | Rule                 | Protocol                 | Port(s)                                                 | IpRange(s)   |
    | -------------------- | ------------------------ | ------------------------------------------------------- | ------------ |
    | Client Traffic       | `spec.ports[*].protocol` | `spec.ports[*].port`                                    | NLB Subnet CIDRs |
    | Health Check Traffic | TCP                      | [Health Check Ports](./annotations.md#healthcheck-port) | NLB Subnet CIDRs |
