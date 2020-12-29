## Service annotations

!!!note ""
    - Annotation keys and values can only be strings. All other types below must be string-encoded, for example:
        - boolean: `"true"`
        - integer: `"42"`
        - stringList: `"s1,s2,s3"`
        - stringMap: `"k1=v1,k2=v2"`
        - json: `"{ \"key\": \"value\" }"`

## Annotations
| Name                                                                           | Type       | Default                   | Notes                  |
|--------------------------------------------------------------------------------|------------|---------------------------|------------------------|
| service.beta.kubernetes.io/aws-load-balancer-type                              | string     |                           |                        |
| service.beta.kubernetes.io/aws-load-balancer-internal                          | boolean    | false                     |                        |
| [service.beta.kubernetes.io/aws-load-balancer-proxy-protocol](#proxy-protocol-v2)                 | string     |        | Set to `"*"` to enable |
| service.beta.kubernetes.io/aws-load-balancer-ip-address-type                   | string     | ipv4                      | ipv4 \| dualstack      |
| service.beta.kubernetes.io/aws-load-balancer-access-log-enabled                | boolean    | false                     |                        |
| service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name         | string     |                           |                        |
| service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix       | string     |                           |                        |
| service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled | boolean    | false                     |                        |
| service.beta.kubernetes.io/aws-load-balancer-ssl-cert                          | stringList |                           |                        |
| service.beta.kubernetes.io/aws-load-balancer-ssl-ports                         | stringList |                           |                        |
| service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy            | string     | ELBSecurityPolicy-2016-08 |                        |
| service.beta.kubernetes.io/aws-load-balancer-backend-protocol                  | string     |                           |                        |
| service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags          | stringMap  |                           |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold     | integer    | 3                         |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold   | integer    | 3                         |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout               | integer    | 10                        |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval              | integer    | 10                        |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol              | string     | TCP                       |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-port                  | integer \| traffic-port | traffic-port              |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-path                  | string     | "/" for HTTP(S) protocols |                        |
| service.beta.kubernetes.io/aws-load-balancer-eip-allocations                   | stringList |                           |                        |
| [service.beta.kubernetes.io/aws-load-balancer-target-group-attributes](#target-group-attributes)  | stringMap  |        |                        |
| [service.beta.kubernetes.io/aws-load-balancer-subnets](#subnets)              | stringList  |                           |                        |
| [service.beta.kubernetes.io/aws-load-balancer-alpn-policy](#alpn-policy)      | stringList  |                           |                        |


## Traffic Routing
Traffic Routing can be controlled with following annotations:

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

    !!!note "requirements"
        TLS listener forwarding to a TLS target group

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

## Resource attributes
NLB target group attributes can be controlled via the following annotations:

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
