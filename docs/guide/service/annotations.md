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
| [service.beta.kubernetes.io/aws-load-balancer-proxy-protocol](#proxy-protocol-v2)                 | string     |                           | Set to `"*"` to enable |
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
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-port                  | string     | traffic-port              |                        |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-path                  | string     | "/" for HTTP(S) protocols |                        |
| service.beta.kubernetes.io/aws-load-balancer-eip-allocations                   | stringList |                           |                        |
| [service.beta.kubernetes.io/aws-load-balancer-target-group-attributes](#target-group-attributes)  | stringMap  |                           |                        |

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
