# Network Load Balancer

The AWS Load Balancer Controller (LBC) supports reconciliation for Kubernetes Service resources of type `LoadBalancer` by provisioning an AWS Network Load Balancer (NLB) with an `instance` or `ip` [target type](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#target-type).

!!! info "Secure by default"
    Since the [:octicons-tag-24: v2.2.0](https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/tag/v2.2.0) release, the LBC provisions an `internal` NLB by default.

    To create an `internet-facing` NLB, the following annotation is required on your service:

    ```yaml
    service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
    ```

    !!! tip ""
        For backwards compatibility, if the [`service.beta.kubernetes.io/aws-load-balancer-scheme`](./annotations.md#lb-scheme) annotation is absent, an existing NLB's scheme remains unchanged.

## Prerequisites
* LBC >= v2.2.0
* For Kubernetes Service resources of type `LoadBalancer`:
    * Kubernetes >= v1.20 or
    * Kubernetes >= v1.19.10 for 1.19 or
    * Kubernetes >= v1.18.18 for 1.18 or
    * EKS >= v1.16
* For Kubernetes Service resources of type `NodePort`:
    * Kubernetes >= v1.16
* For `ip` target type:
    * Pods have native AWS VPC networking configured. For more information, see the [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s#readme) documentation.

## Configuration

By default, Kubernetes Service resources of type `LoadBalancer` get reconciled by the Kubernetes controller built into the `CloudProvider` component of the `kube-controller-manager` or the `cloud-controller-manager`(also known as the in-tree controller).

In order for the LBC to manage the reconciliation of Kubernetes Service resources of type `LoadBalancer`, you need to offload the reconciliation from the in-tree controller to the LBC, explicitly.


=== "With LoadBalancerClass"
    The LBC supports the `LoadBalancerClass` feature since the [:octicons-tag-24: v2.4.0](https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/tag/v2.4.0) release for Kubernetes v1.22+ clusters.

    The `LoadBalancerClass` feature provides a `CloudProvider` agnostic way of offloading the reconciliation for Kubernetes Service resources of type `LoadBalancer` to an external controller.

    When you specify the `spec.loadBalancerClass` to be `service.k8s.aws/nlb` on a Kubernetes Service resource of type `LoadBalancer`, the LBC takes charge of reconciliation by provisioning an NLB.

    !!! warning
        - If you modify a Service resource with matching `spec.loadBalancerClass` by changing its `type` from `LoadBalancer` to anything else, the controller will cleanup the provisioned NLB for that Service.

        - If the `spec.loadBalancerClass` is set to a `loadBalancerClass` that isn't recognized by the LBC, it ignores the Service resource, regardless of the `service.beta.kubernetes.io/aws-load-balancer-type` annotation.

    !!! tip
        - By default, the NLB uses the `instance` target type. You can customize it using the [`service.beta.kubernetes.io/aws-load-balancer-nlb-target-type` annotation](./annotations.md#nlb-target-type).

        - The LBC uses `service.k8s.aws/nlb` as the default `LoadBalancerClass`. You can customize it to a different value using the controller flag `--load-balancer-class`.

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
    The AWS in-tree controller supports an AWS specific way of offloading the reconciliation for Kubernetes Service resources of type `LoadBalancer` to an external controller.

    When you specify the [`service.beta.kubernetes.io/aws-load-balancer-type` annotation](./annotations.md#lb-type) to be `external` on a Kubernetes Service resource of type `LoadBalancer`, the in-tree controller ignores the Service resource. In addition, if you specify the [`service.beta.kubernetes.io/aws-load-balancer-nlb-target-type` annotation](./annotations.md#nlb-target-type) on the Service resource, the LBC takes charge of reconciliation by provisioning an NLB.

    !!! warning
        - It's not recommended to modify or add the `service.beta.kubernetes.io/aws-load-balancer-type` annotation on an existing Service resource. If a change is desired, delete the existing Service resource and create a new one instead of modifying an existing Service.

        - If you modify this annotation on an existing Service resource, you might end up with leaked LBC resources.

    !!! note "backwards compatibility for `nlb-ip` type"
        For backwards compatibility, both the in-tree and LBC controller supports `nlb-ip` as a value for the `service.beta.kubernetes.io/aws-load-balancer-type` annotation. The controllers treats it as if you specified both of the following annotations:
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
The LBC supports both TCP and UDP protocols. The controller also configures TLS termination on your NLB if you configure the Service with a certificate annotation.

In the case of TCP, an NLB with IP targets doesn't pass the client source IP address, unless you specifically configure it to using target group attributes. Your application pods might not see the actual client IP address, even if the NLB passes it along. For example, if you're using instance mode with `externalTrafficPolicy` set to `Cluster`.
In such cases, you can configure [NLB proxy protocol v2](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#proxy-protocol) using an [annotation](https://kubernetes.io/docs/concepts/services-networking/service/#proxy-protocol-support-on-aws) if you need visibility into
the client source IP address on your application pods.

To enable proxy protocol v2, apply the following annotation to your Service:
```yaml
service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: "*"
```
!!!note ""
    If you enable proxy protocol v2, NLB health checks with HTTP/HTTPS only work if the health check port supports proxy protocol v2. Due to this behavior, you shouldn't configure proxy protocol v2 with NLB instance mode and `externalTrafficPolicy` set to `Local`.

## Subnet tagging requirements
See [Subnet Discovery](../../deploy/subnet_discovery.md) for details on configuring Elastic Load Balancing for public or private placement.

## Security group
 - From v2.6.0, the AWS LBC creates and attaches frontend and backend security groups to NLB by default. For more information please see [the security groups documentation](../../deploy/security_groups.md) 
 - In older versions, the controller by default adds inbound rules to the worker node security groups, to allow inbound traffic from an NLB.

!!! tip "disable worker node security group rule management"
    You can disable the worker node security group rule management using an [annotation](./annotations.md#manage-backend-sg-rules).

### Worker node security groups selection
The controller automatically selects the worker node security groups that it modifies to allow inbound traffic using the following rules:

* For `instance` mode, the security group of each backend worker node's primary elastic network interface (ENI) is selected.
* For `ip` mode, the security group of each backend pod's ENI is selected.

!!! important "Multiple security groups on an ENI"

    If there are multiple security groups attached to an ENI, the controller expects only one security group tagged with following tags:

    | Key                                     | Value               |
    | --------------------------------------- | ------------------- |
    | `kubernetes.io/cluster/${cluster-name}` | `owned` or `shared` |

    `${cluster-name}` is the name of the Kubernetes cluster.

If it is possible for multiple security groups with the tag `kubernetes.io/cluster/${cluster-name}` to be on a target ENI, you may use the `--service-target-eni-security-group-tags` flag to specify additional tags that must also match in order for a security group to be used.

### Worker node security groups rules

=== "When client IP preservation is enabled"

    | Rule                 | Protocol                 | Port(s)                                                 | IpRanges(s)                                         |
    | -------------------- | ------------------------ | ------------------------------------------------------- | --------------------------------------------------- |
    | Client Traffic       | `spec.ports[*].protocol` | `spec.ports[*].port`                                    | [Traffic Source CIDRs](./annotations.md#lb-source-ranges) |
    | Health Check Traffic | TCP                      | [Health Check Ports](./annotations.md#healthcheck-port) | NLB Subnet CIDRs                                        |

=== "When client IP preservation is disabled"

    | Rule                 | Protocol                 | Port(s)                                                 | IpRange(s)   |
    | -------------------- | ------------------------ | ------------------------------------------------------- | ------------ |
    | Client Traffic       | `spec.ports[*].protocol` | `spec.ports[*].port`                                    | NLB Subnet CIDRs |
    | Health Check Traffic | TCP                      | [Health Check Ports](./annotations.md#healthcheck-port) | NLB Subnet CIDRs |
