## Service annotations

!!!note ""
    - Annotation keys and values can only be strings. Advanced format should be encoded as below:
        - boolean: 'true'
        - integer: '42'
        - stringList: s1,s2,s3
        - stringMap: k1=v1,k2=v2
        - json: 'jsonContent'

## Annotations
|Name                                                                               | Type              | Default       |
|-----------------------------------------------------------------------------------|-------------------|---------------|
|service.beta.kubernetes.io/aws-load-balancer-type                                  | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-internal                              | boolean           | false         |
|service.beta.kubernetes.io/aws-load-balancer-proxy-protocol                        | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-access-log-enabled                    | boolean           | false         |
|service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name             | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix           | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled     | boolean           | false         |
|service.beta.kubernetes.io/aws-load-balancer-ssl-cert                              | stringList        |               |
|service.beta.kubernetes.io/aws-load-balancer-ssl-ports                             | stringList        |               |
|service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy                | string            | ELBSecurityPolicy-2016-08            |
|service.beta.kubernetes.io/aws-load-balancer-backend-protocol                      | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags              | stringMap         |               |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold         | integer           | 3             |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold       | integer           | 3             |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout                   | integer           | 10            |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval                  | integer           | 10            |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol                  | string            | TCP           |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-port                      | string            | traffic-port  |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-path                      | string            | "/" for HTTP(S) protocols |
|service.beta.kubernetes.io/aws-load-balancer-eip-allocations			            | stringList	    |		|
