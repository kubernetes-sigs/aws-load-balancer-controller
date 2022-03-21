## Service annotations

!!!note ""
    - Annotation keys and values can only be strings. All other types below must be string-encoded, for example:
        - boolean: `"true"`
        - integer: `"42"`
        - stringList: `"s1,s2,s3"`
        - stringMap: `"k1=v1,k2=v2"`
        - json: `"{ \"key\": \"value\" }"`

## Annotations
!!!warning
    These annotations are specific to the kubernetes [service resources reconciled](#lb-type) by the AWS Load Balancer Controller. Although the list was initially derived from the k8s in-tree `kube-controller-manager`, this
    documentation is not an accurate reference for the services reconciled by the in-tree controller. 

| Name                                                                                             | Type                    | Default                   | Notes                                                  |
|--------------------------------------------------------------------------------------------------|-------------------------|---------------------------|--------------------------------------------------------|
| [service.beta.kubernetes.io/load-balancer-source-ranges](#lb-source-ranges)                      | stringList              |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-type](#lb-type)                                    | string                  |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-nlb-target-type](#nlb-target-type)                 | string                  |                           | default `instance` in case of LoadBalancerClass        |
| [service.beta.kubernetes.io/aws-load-balancer-name](#load-balancer-name)                         | string                  |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-internal](#lb-internal)                            | boolean                 | false                     | deprecated, in favor of [aws-load-balancer-scheme](#lb-scheme)|
| [service.beta.kubernetes.io/aws-load-balancer-scheme](#lb-scheme)                                | string                  | internal                  |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-proxy-protocol](#proxy-protocol-v2)                | string                  |                           | Set to `"*"` to enable                                 |
| [service.beta.kubernetes.io/aws-load-balancer-ip-address-type](#ip-address-type)                 | string                  | ipv4                      | ipv4 \| dualstack                                      |
| [service.beta.kubernetes.io/aws-load-balancer-access-log-enabled](#deprecated-attributes)        | boolean                 | false                     | deprecated, in favor of [aws-load-balancer-attributes](#load-balancer-attributes)|
| [service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name](#deprecated-attributes) | string                  |                           | deprecated, in favor of [aws-load-balancer-attributes](#load-balancer-attributes)|
| [service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix](#deprecated-attributes)| string                 |                           | deprecated, in favor of [aws-load-balancer-attributes](#load-balancer-attributes)|
| [service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled](#deprecated-attributes)| boolean          | false                     | deprecated, in favor of [aws-load-balancer-attributes](#load-balancer-attributes)|
| [service.beta.kubernetes.io/aws-load-balancer-ssl-cert](#ssl-cert)                               | stringList              |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-ssl-ports](#ssl-ports)                             | stringList              |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy](#ssl-negotiation-policy)   | string                  | ELBSecurityPolicy-2016-08 |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-backend-protocol](#backend-protocol)               | string                  |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags](#additional-resource-tags) | stringMap             |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol](#healthcheck-protocol)       | string                  | TCP                       |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-healthcheck-port ](#healthcheck-port)              | integer \| traffic-port | traffic-port              |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-healthcheck-path](#healthcheck-path)               | string                  | "/" for HTTP(S) protocols |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold](#healthcheck-healthy-threshold)     | integer | 3                         |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold](#healthcheck-unhealthy-threshold) | integer | 3                         |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout](#healthcheck-timeout)         | integer                 | 10                        |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval](#healthcheck-interval)       | integer                 | 10                        |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-eip-allocations](#eip-allocations)                 | stringList              |                           | internet-facing lb only. Length must match the number of subnets|
| [service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses](#private-ipv4-addresses)   | stringList              |                           | internal lb only. Length must match the number of subnets |
| [service.beta.kubernetes.io/aws-load-balancer-target-group-attributes](#target-group-attributes) | stringMap               |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-subnets](#subnets)                                 | stringList              |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-alpn-policy](#alpn-policy)                         | stringList              |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-target-node-labels](#target-node-labels)           | stringMap               |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-attributes](#load-balancer-attributes)             | stringMap               |                           |                                                        |
| [service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules](#manage-backend-sg-rules)  | boolean    | true                      |                                                        |

## Traffic Routing
Traffic Routing can be controlled with following annotations:

- <a name="load-balancer-name">`service.beta.kubernetes.io/aws-load-balancer-name`</a> specifies the custom name to use for the load balancer. Name longer than 32 characters will be treated as an error.

    !!!note "limitations"
        - If you modify this annotation after service creation, there is no effect.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-name: custom-name
        ```

- <a name="lb-type">`service.beta.kubernetes.io/aws-load-balancer-type`</a> specifies the load balancer type. This controller reconciles those service resources with this annotation set to either `nlb-ip` or `external`.

    !!!tip
        This annotation specifies the controller used to provision LoadBalancers (as specified in [legacy-cloud-provider](#legacy-cloud-provider)). Refer to [lb-scheme](#lb-scheme) to specify whether the LoadBalancer is internet-facing or internal.
    
    !!!note ""
        - [Deprecated] For type `nlb-ip`, the controller will provision an NLB with targets registered by IP address. This value is supported for backwards compatibility.
        - For type `external`, the NLB target type depends on the [nlb-target-type](#nlb-target-type) annotation.

    !!!warning "limitations"
        - This annotation should not be modified after service creation.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-type: external
        ```

- <a name="nlb-target-type">`service.beta.kubernetes.io/aws-load-balancer-nlb-target-type`</a> specifies the target type to configure for NLB. You can choose between
`instance` and `ip`.
    - `instance` mode will route traffic to all EC2 instances within cluster on the [NodePort](https://kubernetes.io/docs/concepts/services-networking/service/#type-nodeport) opened for your service.

        !!!note ""
            - service must be of type `NodePort` or `LoadBalancer` for `instance` targets
            - for k8s 1.22 and later if `spec.allocateLoadBalancerNodePorts` is set to `false`, `NodePort` must be allocated manually

        !!!note "default value"
            If you configure `spec.loadBalancerClass`, the controller defaults to `instance` target type

      - `ip` mode will route traffic directly to the pod IP.

        !!!note ""
            network plugin must use native AWS VPC networking configuration for pod IP, for example [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s).

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: instance
        ```

- <a name="subnets">`service.beta.kubernetes.io/aws-load-balancer-subnets`</a> specifies the [Availability Zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html)
the NLB will route traffic to. See [Network Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/network-load-balancers.html#availability-zones) for more details.

    !!!tip
        Subnets are auto-discovered if this annotation is not specified, see [Subnet Discovery](../../deploy/subnet_discovery.md) for further details.

    !!!note ""
        You must specify at least one subnet in any of the AZs, both subnetID or subnetName(Name tag on subnets) can be used.

    !!!warning "limitations"
        - Each subnets must be from a different Availability Zone
        - AWS has restrictions on disabling existing subnets for NLB. As a result, you might not be able to edit this annotation once the NLB gets provisioned.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-subnets: subnet-xxxx, mySubnet
        ```

- <a name="alpn-policy">`service.beta.kubernetes.io/aws-load-balancer-alpn-policy`</a> allows you to configure the [ALPN policies](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/create-tls-listener.html#alpn-policies)
on the load balancer.

    !!!tip "supported policies"
        - `HTTP1Only` Negotiate only HTTP/1.*. The ALPN preference list is http/1.1, http/1.0.
        - `HTTP2Only` Negotiate only HTTP/2. The ALPN preference list is h2.
        - `HTTP2Optional` Prefer HTTP/1.* over HTTP/2 (which can be useful for HTTP/2 testing). The ALPN preference list is http/1.1, http/1.0, h2.
        - `HTTP2Preferred` Prefer HTTP/2 over HTTP/1.*. The ALPN preference list is h2, http/1.1, http/1.0.
        - `None` Do not negotiate ALPN. This is the default.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-alpn-policy: HTTP2Preferred
        ```

- <a name="target-node-labels">`service.beta.kubernetes.io/aws-load-balancer-target-node-labels`</a> specifies which nodes to include in the target group registration for `instance` target type.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-target-node-labels: label1=value1, label2=value2
        ```

- <a name="eip-allocations">`service.beta.kubernetes.io/aws-load-balancer-eip-allocations`</a> specifies a list of [elastic IP address](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html) configuration for an internet-facing NLB.

    !!!note
        - This configuration is optional and use can use it to assign static IP addresses to your NLB
        - You must specify the same number of eip allocations as load balancer subnets [annotation](#subnets)
        - NLB must be internet-facing

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-eip-allocations: eipalloc-xyz, eipalloc-zzz
        ```


- <a name="private-ipv4-addresses">`service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses`</a> specifies a list of private IPv4 addresses for an internal NLB.

    !!!note
        - NLB must be internal
        - This configuration is optional and use can use it to assign static IP addresses to your NLB
        - You must specify the same number of private IPv4 addresses as load balancer subnets [annotation](#subnets)
        - You must specify the IPv4 addresses from the load balancer subnet IPv4 ranges

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses: 192.168.10.15, 192.168.32.16
        ```

## Traffic Listening
Traffic Listening can be controlled with following annotations:

- <a name="ip-address-type">`service.beta.kubernetes.io/aws-load-balancer-ip-address-type`</a> specifies the [IP address type](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/network-load-balancers.html#ip-address-type) of NLB.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-ip-address-type: ipv4
        ```

## Resource attributes
NLB resource attributes can be controlled via the following annotations:

- <a name="proxy-protocol-v2">service.beta.kubernetes.io/aws-load-balancer-proxy-protocol</a> specifies whether to enable proxy protocol v2 on the target group.
Set to '*' to enable proxy protocol v2. This annotation takes precedence over the annotation `service.beta.kubernetes.io/aws-load-balancer-target-group-attributes`
for proxy protocol v2 configuration.

    !!!note ""
        The only valid value for this annotation is `*`.

- <a name="target-group-attributes">`service.beta.kubernetes.io/aws-load-balancer-target-group-attributes`</a> specifies the
[Target Group Attributes](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#target-group-attributes) to be configured.

    !!!example
        - set the deregistration delay to 120 seconds (available range is 0-3600 seconds)
            ```
            service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: deregistration_delay.timeout_seconds=120
            ```
        - enable source IP affinity
            ```
            service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: stickiness.enabled=true,stickiness.type=source_ip
            ```
        - enable proxy protocol version 2
            ```
            service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: proxy_protocol_v2.enabled=true
            ```
        - enable connection termination on deregistration
            ```
            service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: deregistration_delay.connection_termination.enabled=true
            ```
        - enable [client IP preservation](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#client-ip-preservation)
            ```
            service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: preserve_client_ip.enabled=true
            ```


- <a name="load-balancer-attributes">`service.beta.kubernetes.io/aws-load-balancer-attributes`</a> specifies [Load Balancer Attributes](http://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_LoadBalancerAttribute.html) that should be applied to the NLB.

    !!!warning ""
        Only attributes defined in the annotation will be updated. To unset any AWS defaults(e.g. Disabling access logs after having them enabled once), the values need to be explicitly set to the original values(`access_logs.s3.enabled=false`) and omitting them is not sufficient.
        Custom attributes set in this annotation's config map will be overriden by annotation-specific attributes. For backwards compatibility, existing annotations for the individual load balancer attributes get precedence in case of ties.
  
    !!!note ""
        - If `deletion_protection.enabled=true` is in the annotation, the controller will not be able to delete the NLB during reconciliation. Once the attribute gets edited to `deletion_protection.enabled=false` during reconciliation, the deployer will force delete the resource.
        - Please note, if the deletion protection is not enabled via annotation (e.g. via AWS console), the controller still deletes the underlying resource.
    
    !!!example
        - enable access log to s3
        ```
        service.beta.kubernetes.io/aws-load-balancer-attributes: access_logs.s3.enabled=true,access_logs.s3.bucket=my-access-log-bucket,access_logs.s3.prefix=my-app
        ```
        - enable NLB deletion protection
        ```
        service.beta.kubernetes.io/aws-load-balancer-attributes: deletion_protection.enabled=true
        ```
        - enable cross zone load balancing
        ```
        service.beta.kubernetes.io/aws-load-balancer-attributes: load_balancing.cross_zone.enabled=true
        ```

- <a name="deprecated-attributes"></a>the following annotations are deprecated in v2.3.0 release in favor of [service.beta.kubernetes.io/aws-load-balancer-attributes](#load-balancer-attributes)

    !!!note ""
        ```
        service.beta.kubernetes.io/aws-load-balancer-access-log-enabled
        service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name
        service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix
        service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled 
        ```


## AWS Resource Tags
The AWS Load Balancer Controller automatically applies following tags to the AWS resources it creates (NLB/TargetGroups/Listener/ListenerRule):

- `elbv2.k8s.aws/cluster: ${clusterName}`
- `service.k8s.aws/stack: ${stackID}`
- `service.k8s.aws/resource: ${resourceID}`

In addition, you can use annotations to specify additional tags

- <a name="additional-resource-tags">`service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags`</a> specifies additional tags to apply to the AWS resources.

    !!!note ""
        - you cannot override the default controller tags mentioned above or the tags specified in the `--default-tags` controller flag
        - if any of the tag conflicts with the ones configured via `--external-managed-tags` controller flag, the controller fails to reconcile the service

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags: Environment=dev,Team=test
        ```

## Health Check
Health check on target groups can be configured with following annotations:

- <a name="healthcheck-protocol">`service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol`</a> specifies the target group health check protocol.

    !!!note ""
        - you can specify `tcp`, or `http` or `https`, `tcp` is the default
        - `tcp` is the default health check protocol if the service `spec.externalTrafficPolicy` is `Cluster`, `http` if `Local`
        - if the service `spec.externalTrafficPolicy` is `Local`, do **not** use `tcp` for health check

    !!!example
        ```service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol: http
        ```

- <a name="healthcheck-port">`service.beta.kubernetes.io/aws-load-balancer-healthcheck-port`</a> specifies the TCP port to use for target group health check.

    !!!note ""
        - if you do not specify the health check port, controller uses `traffic-port` as default value

    !!!example
        - set the health check port to `traffic-port`
            ```
            service.beta.kubernetes.io/aws-load-balancer-healthcheck-port: traffic-port
            ```
        - set the health check port to port `80`
            ```
            service.beta.kubernetes.io/aws-load-balancer-healthcheck-port: "80"
            ```

- <a name="healthcheck-path">`service.beta.kubernetes.io/aws-load-balancer-healthcheck-path`</a> specifies the http path for the health check in case of http/https protocol.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-healthcheck-path: /healthz
        ```

- <a name="healthcheck-healthy-threshold">`service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold`</a> specifies the consecutive health check successes required before a target is considered healthy.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold: "3"
        ```

- <a name="healthcheck-unhealthy-threshold">`service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold`</a> specifies the consecutive health check failures before a target gets marked unhealthy.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold: "3"
        ```

- <a name="healthcheck-interval">`service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval`</a> specifies the interval between consecutive health checks.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval: "10"
        ```


- <a name="healthcheck-timeout">`service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout`</a> specifies the target group health check timeout. The target has to respond within the timeout for a successful health check.

    !!!note
        The controller currently ignores the timeout configuration due to the limitations on the AWS NLB. The default timeout for TCP is 10s and HTTP is 6s.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout: "10"
        ```

## TLS
You can configure TLS support via the following annotations:

- <a name="ssl-cert">`service.beta.kubernetes.io/aws-load-balancer-ssl-cert`</a> specifies the ARN of one or more certificates managed by the [AWS Certificate Manager](https://aws.amazon.com/certificate-manager).

    !!!note ""
        The first certificate in the list is the default certificate and remaining certificates are for the optional certificate list.
        See [Server Certificates](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/create-tls-listener.html#tls-listener-certificates) for further details.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-ssl-cert: arn:aws:acm:us-west-2:xxxxx:certificate/xxxxxxx
        ```

- <a name="ssl-ports">`service.beta.kubernetes.io/aws-load-balancer-ssl-ports`</a> specifies the frontend ports with TLS listeners.

    !!!note ""
        - You must configure at least one [certificate](#ssl-cert) for TLS listeners
        - You can specify a list of port names or port values, `*` does **not** match any ports
        - If you don't specify this annotation, controller creates TLS listener for all the service ports
        - Specify this annotation if you need both TLS and non-TLS listeners on the same load balancer

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-ssl-ports: 443, custom-port
        ```

- <a name="ssl-negotiation-policy">`service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy`</a> specifies the [Security Policy](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/create-tls-listener.html#describe-ssl-policies) for NLB frontend connections, allowing you to control the protocol and ciphers.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy: ELBSecurityPolicy-TLS13-1-2-2021-06
        ```

- <a name="backend-protocol">`service.beta.kubernetes.io/aws-load-balancer-backend-protocol`</a> specifies whether to use TLS for the backend traffic between the load balancer and the kubernetes pods.

    !!!note ""
        - If you specify `ssl` as the backend protocol, NLB uses TLS connections for the traffic to your kubernetes pods in case of TLS listeners
        - You can specify `ssl` or `tcp` (default)

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-backend-protocol: ssl
        ```


## Access control
Load balancer access can be controlled via following annotations:

- <a name="lb-source-ranges">`service.beta.kubernetes.io/load-balancer-source-ranges`</a> specifies the CIDRs that are allowed to access the NLB.

    !!!tip
        we recommend specifying CIDRs in the service `Spec.LoadBalancerSourceRanges` instead

    !!!note "Default"
        - `0.0.0.0/0` will be used if the IPAddressType is "ipv4"
        - `0.0.0.0/0` and `::/0` will be used if the IPAddressType is "dualstack"
        - The VPC CIDR will be used if `service.beta.kubernetes.io/aws-load-balancer-scheme` is `internal`

    !!!warning ""
        This annotation will be ignored in case preserve client IP is not enabled.
        - preserve client IP is disabled by default for `IP` targets
        - preserve client IP is enabled by default for `instance` targets

    !!!example
        ```
        service.beta.kubernetes.io/load-balancer-source-ranges: 10.0.0.0/24
        ```

- <a name="lb-scheme">`service.beta.kubernetes.io/aws-load-balancer-scheme`</a> specifies whether the NLB will be internet-facing or internal.  Valid values are `internal`, `internet-facing`. If not specified, default is `internal`.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"
        ```

- <a name="lb-internal">`service.beta.kubernetes.io/aws-load-balancer-internal`</a> specifies whether the NLB will be internet-facing or internal.

    !!!note "deprecation note"
        This annotation is deprecated starting v2.2.0 release in favor of the new [aws-load-balancer-scheme](#lb-scheme) annotation. It will be supported, but in case of ties, the aws-load-balancer-scheme gets precedence.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-internal: "true"
        ```

- <a name="manage-backend-sg-rules">`service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules`</a> specifies whether the controller should automatically add the ingress rules to the instance/ENI security group.

    !!!warning ""
        If you disable the automatic management of security group rules for an NLB, you will need to manually add appropriate ingress rules to your EC2 instance or ENI security groups to allow access to the traffic and health check ports.

    !!!example
        ```
        service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules: "false"
        ```

## Legacy Cloud Provider
The AWS Load Balancer Controller manages Kubernetes Services in a compatible way with the legacy aws cloud provider. The annotation `service.beta.kubernetes.io/aws-load-balancer-type` is used to determine which controller reconciles the service. If the annotation value is `nlb-ip` or `external`, legacy cloud provider ignores the service resource (provided it has the correct patch) so that the AWS Load Balancer controller can take over. For all other values of the annotation, the legacy cloud provider will handle the service. Note that this annotation should be specified during service creation and not edited later.

The legacy cloud provider patch was added in Kubernetes v1.20 and is backported to Kubernetes v1.18.18+, v1.19.10+.
