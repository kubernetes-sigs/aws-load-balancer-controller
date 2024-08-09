#  Ingress annotations
You can add annotations to kubernetes Ingress and Service objects to customize their behavior.

!!!note ""
    - Annotation keys and values can only be strings. Advanced format should be encoded as below:
        - boolean: 'true'
        - integer: '42'
        - stringList: s1,s2,s3
        - stringMap: k1=v1,k2=v2
        - json: 'jsonContent'
    - Annotations applied to Service have higher priority over annotations applied to Ingress. `Location` column below indicates where that annotation can be applied to.
    - Annotations that configures LoadBalancer / Listener behaviors have different merge behavior when IngressGroup feature is been used. `MergeBehavior` column below indicates how such annotation will be merged.
        - Exclusive: such annotation should only be specified on a single Ingress within IngressGroup or specified with same value across all Ingresses within IngressGroup.
        - Merge: such annotation can be specified on all Ingresses within IngressGroup, and will be merged together.

## Annotations
| Name                                                                                                  | Type                        |Default| Location        | MergeBehavior |
|-------------------------------------------------------------------------------------------------------|-----------------------------|------|-----------------|-----------|
| [alb.ingress.kubernetes.io/load-balancer-name](#load-balancer-name)                                   | string                      |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/group.name](#group.name)                                                   | string                      |N/A| Ingress         | N/A       |
| [alb.ingress.kubernetes.io/group.order](#group.order)                                                 | integer                     |0| Ingress         | N/A       |
| [alb.ingress.kubernetes.io/tags](#tags)                                                               | stringMap                   |N/A| Ingress,Service | Merge     |
| [alb.ingress.kubernetes.io/ip-address-type](#ip-address-type)                                         | ipv4 \| dualstack \|  dualstack-without-public-ipv4           |ipv4| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/scheme](#scheme)                                                           | internal \| internet-facing |internal| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/subnets](#subnets)                                                         | stringList                  |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/security-groups](#security-groups)                                         | stringList                  |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/manage-backend-security-group-rules](#manage-backend-security-group-rules) | boolean                     |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/customer-owned-ipv4-pool](#customer-owned-ipv4-pool)                       | string                      |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/load-balancer-attributes](#load-balancer-attributes)                       | stringMap                   |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/wafv2-acl-arn](#wafv2-acl-arn)                                             | string                      |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/waf-acl-id](#waf-acl-id)                                                   | string                      |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/shield-advanced-protection](#shield-advanced-protection)                   | boolean                     |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/listen-ports](#listen-ports)                                               | json                        |'[{"HTTP": 80}]' \| '[{"HTTPS": 443}]'| Ingress         | Merge     |
| [alb.ingress.kubernetes.io/ssl-redirect](#ssl-redirect)                                               | integer                     |N/A| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/inbound-cidrs](#inbound-cidrs)                                             | stringList                  |0.0.0.0/0, ::/0| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/security-group-prefix-lists](#security-group-prefix-lists)                                               | stringList                        |pl-00000000, pl-1111111| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/certificate-arn](#certificate-arn)                                         | stringList                  |N/A| Ingress         | Merge     |
| [alb.ingress.kubernetes.io/ssl-policy](#ssl-policy)                                                   | string                      |ELBSecurityPolicy-2016-08| Ingress         | Exclusive |
| [alb.ingress.kubernetes.io/target-type](#target-type)                                                 | instance \| ip              |instance| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/backend-protocol](#backend-protocol)                                       | HTTP \| HTTPS               |HTTP| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/backend-protocol-version](#backend-protocol-version)                       | string                      | HTTP1 | Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/target-group-attributes](#target-group-attributes)                         | stringMap                   |N/A| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/healthcheck-port](#healthcheck-port)                                       | integer \| traffic-port     |traffic-port| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/healthcheck-protocol](#healthcheck-protocol)                               | HTTP \| HTTPS               |HTTP| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/healthcheck-path](#healthcheck-path)                                       | string                      |/ \| /AWS.ALB/healthcheck | Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/healthcheck-interval-seconds](#healthcheck-interval-seconds)               | integer                     |'15'| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/healthcheck-timeout-seconds](#healthcheck-timeout-seconds)                 | integer                     |'5'| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/healthy-threshold-count](#healthy-threshold-count)                         | integer                     |'2'| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/unhealthy-threshold-count](#unhealthy-threshold-count)                     | integer                     |'2'| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/success-codes](#success-codes)                                             | string                      |'200' \| '12' | Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/auth-type](#auth-type)                                                     | none\|oidc\|cognito         |none| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/auth-idp-cognito](#auth-idp-cognito)                                       | json                        |N/A| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/auth-idp-oidc](#auth-idp-oidc)                                             | json                        |N/A| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/auth-on-unauthenticated-request](#auth-on-unauthenticated-request)         | authenticate\|allow\|deny   |authenticate| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/auth-scope](#auth-scope)                                                   | string                      |openid| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/auth-session-cookie](#auth-session-cookie)                                 | string                      |AWSELBAuthSessionCookie| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/auth-session-timeout](#auth-session-timeout)                               | integer                     |'604800'| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/actions.${action-name}](#actions)                                          | json                        |N/A| Ingress         | N/A       |
| [alb.ingress.kubernetes.io/conditions.${conditions-name}](#conditions)                                | json                        |N/A| Ingress         | N/A       |
| [alb.ingress.kubernetes.io/target-node-labels](#target-node-labels)                                   | stringMap                   |N/A| Ingress,Service | N/A       |
| [alb.ingress.kubernetes.io/mutual-authentication](#mutual-authentication)                             | json                        |N/A| Ingress         |Exclusive|

## IngressGroup
IngressGroup feature enables you to group multiple Ingress resources together.
The controller will automatically merge Ingress rules for all Ingresses within IngressGroup and support them with a single ALB.
In addition, most annotations defined on an Ingress only apply to the paths defined by that Ingress.

By default, Ingresses don't belong to any IngressGroup, and we treat it as a "implicit IngressGroup" consisting of the Ingress itself.

- <a name="group.name">`alb.ingress.kubernetes.io/group.name`</a> specifies the group name that this Ingress belongs to.

    !!!note ""
        - Ingresses with same `group.name` annotation will form an "explicit IngressGroup".
        - groupName must consist of lower case alphanumeric characters, `-` or `.`, and must start and end with an alphanumeric character.
        - groupName must be no more than 63 character.

    !!!warning "Security Risk"
        IngressGroup feature should only be used when all Kubernetes users with RBAC permission to create/modify Ingress resources are within trust boundary.

        If you turn your Ingress to belong a "explicit IngressGroup" by adding `group.name` annotation,
        other Kubernetes users may create/modify their Ingresses to belong to the same IngressGroup, and can thus add more rules or overwrite existing rules with higher priority to the ALB for your Ingress.

        We'll add more fine-grained access-control in future versions.
  
    !!!note "Rename behavior"
        The ALB for an IngressGroup is found by searching for an AWS tag `ingress.k8s.aws/stack` tag with the name of the IngressGroup as its value. For an implicit IngressGroup, the value is `namespace/ingressname`.

        When the groupName of an IngressGroup for an Ingress is changed, the Ingress will be moved to a new IngressGroup and be supported by the ALB for the new IngressGroup. If the ALB for the new IngressGroup doesn't exist, a new ALB will be created.

        If an IngressGroup no longer contains any Ingresses, the ALB for that IngressGroup will be deleted and any deletion protection of that ALB will be ignored.

    !!!example
        ```
        alb.ingress.kubernetes.io/group.name: my-team.awesome-group
        ```

- <a name="group.order">`alb.ingress.kubernetes.io/group.order`</a> specifies the order across all Ingresses within IngressGroup.

    !!!note ""
        - You can explicitly denote the order using a number between -1000 and 1000
        - The smaller the order, the rule will be evaluated first. All Ingresses without an explicit order setting get order value as 0
        - Rules with the same order are sorted lexicographically by the Ingress’s namespace/name.

    !!!example
        ```
        alb.ingress.kubernetes.io/group.order: '10'
        ```

## Traffic Listening
Traffic Listening can be controlled with the following annotations:

- <a name="listen-ports">`alb.ingress.kubernetes.io/listen-ports`</a> specifies the ports that ALB listens on.

    !!!note "Merge Behavior"
        `listen-ports` is merged across all Ingresses in IngressGroup.

        - You can define different listen-ports per Ingress, Ingress rules will only impact the ports defined for that Ingress.
        - If same listen-port is defined by multiple Ingress within IngressGroup, Ingress rules will be merged with respect to their group order within IngressGroup.

    !!!note "Default"
        - defaults to `'[{"HTTP": 80}]'` or `'[{"HTTPS": 443}]'` depending on whether `certificate-arn` is specified.

    !!!warning ""
        You may not have duplicate load balancer ports defined.

    !!!example
        ```
        alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}, {"HTTP": 8080}, {"HTTPS": 8443}]'
        ```

- <a name="ssl-redirect">`alb.ingress.kubernetes.io/ssl-redirect`</a> enables SSLRedirect and specifies the SSL port that redirects to.

    !!!note "Merge Behavior"
        `ssl-redirect` is exclusive across all Ingresses in IngressGroup.

        - Once defined on a single Ingress, it impacts every Ingress within IngressGroup.

    !!!note ""
        - Once enabled SSLRedirect, every HTTP listener will be configured with a default action which redirects to HTTPS, other rules will be ignored.
        - The SSL port that redirects to must exists on LoadBalancer. See [alb.ingress.kubernetes.io/listen-ports](#listen-ports) for the listen ports configuration.

    !!!example
        ```
        alb.ingress.kubernetes.io/ssl-redirect: '443'
        ```

- <a name="ip-address-type">`alb.ingress.kubernetes.io/ip-address-type`</a> specifies the [IP address type](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html#ip-address-type) of ALB.

    !!!example
        ```
        alb.ingress.kubernetes.io/ip-address-type: ipv4
        ```

- <a name="customer-owned-ipv4-pool">`alb.ingress.kubernetes.io/customer-owned-ipv4-pool`</a> specifies the customer-owned IPv4 address pool for ALB on Outpost.

    !!!warning ""
        This annotation should be treated as immutable. To remove or change coIPv4Pool, you need to recreate Ingress.

    !!!example
        ```
        alb.ingress.kubernetes.io/customer-owned-ipv4-pool: ipv4pool-coip-xxxxxxxx
        ```

## Traffic Routing
Traffic Routing can be controlled with following annotations:

- <a name="load-balancer-name">`alb.ingress.kubernetes.io/load-balancer-name`</a> specifies the custom name to use for the load balancer. Name longer than 32 characters will be treated as an error.

    !!!note "Merge Behavior"
        `name` is exclusive across all Ingresses in an IngressGroup.

        - Once defined on a single Ingress, it impacts every Ingress within the IngressGroup.

    !!!example
        ```
        alb.ingress.kubernetes.io/load-balancer-name: custom-name
        ```

- <a name="target-type">`alb.ingress.kubernetes.io/target-type`</a> specifies how to route traffic to pods. You can choose between `instance` and `ip`:

    - `instance` mode will route traffic to all ec2 instances within cluster on [NodePort](https://kubernetes.io/docs/concepts/services-networking/service/#type-nodeport) opened for your service.

        !!!note ""
            service must be of type "NodePort" or "LoadBalancer" to use `instance` mode

    - `ip` mode will route traffic directly to the pod IP.

        !!!note ""
            network plugin must use secondary IP addresses on [ENI](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-eni.html) for pod IP to use `ip` mode. e.g.

            - [amazon-vpc-cni-k8s](https://github.com/aws/amazon-vpc-cni-k8s)

        !!!note ""
            `ip` mode is required for sticky sessions to work with Application Load Balancers. The Service type does not matter, when using `ip` mode.

    !!!example
        ```
        alb.ingress.kubernetes.io/target-type: instance
        ```

- <a name="target-node-labels">`alb.ingress.kubernetes.io/target-node-labels`</a> specifies which nodes to include in the target group registration for `instance` target type.

    !!!example
        ```
        alb.ingress.kubernetes.io/target-node-labels: label1=value1, label2=value2
        ```

- <a name="backend-protocol">`alb.ingress.kubernetes.io/backend-protocol`</a> specifies the protocol used when route traffic to pods.

    !!!example
        ```
        alb.ingress.kubernetes.io/backend-protocol: HTTPS
        ```

- <a name="backend-protocol-version">`alb.ingress.kubernetes.io/backend-protocol-version`</a> specifies the application protocol used to route traffic to pods. Only valid when HTTP or HTTPS is used as the backend protocol.

    !!!example
        - HTTP2
            ```
            alb.ingress.kubernetes.io/backend-protocol-version: HTTP2
            ```
        - GRPC
            ```
            alb.ingress.kubernetes.io/backend-protocol-version: GRPC
            ```

- <a name="subnets">`alb.ingress.kubernetes.io/subnets`</a> specifies the [Availability Zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html)s that the ALB will route traffic to. See [Load Balancer subnets](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-subnets.html) for more details.

    !!!note ""
        You must specify at least two subnets in different AZs unless utilizing the outpost locale, in which case a single subnet suffices. Either subnetID or subnetName(Name tag on subnets) can be used.

    !!!note ""
        You must not mix subnets from different locales: availability-zone, local-zone, wavelength-zone, outpost.

    !!!tip
        You can enable subnet auto discovery to avoid specifying this annotation on every Ingress. See [Subnet Discovery](../../deploy/subnet_discovery.md) for instructions.

    !!!example
        ```
        alb.ingress.kubernetes.io/subnets: subnet-xxxx, mySubnet
        ```

- <a name="actions">`alb.ingress.kubernetes.io/actions.${action-name}`</a> Provides a method for configuring custom actions on a listener, such as Redirect Actions.

    The `action-name` in the annotation must match the serviceName in the Ingress rules, and servicePort must be `use-annotation`.

    !!!note "use ARN in forward Action"
        ARN can be used in forward action(both simplified schema and advanced schema), it must be an targetGroup created outside of k8s, typically an targetGroup for legacy application.
    !!!note "use ServiceName/ServicePort in forward Action"
        ServiceName/ServicePort can be used in forward action(advanced schema only).

    !!!warning ""
        [Auth related annotations](#authentication) on Service object will only be respected if a single TargetGroup in is used.

    !!!example
        - response-503: return fixed 503 response
        - redirect-to-eks: redirect to an external url
        - forward-single-tg: forward to a single targetGroup [**simplified schema**]
        - forward-multiple-tg: forward to multiple targetGroups with different weights and stickiness config [**advanced schema**]

        ```yaml
        apiVersion: networking.k8s.io/v1
        kind: Ingress
        metadata:
          namespace: default
          name: ingress
          annotations:
            alb.ingress.kubernetes.io/scheme: internet-facing
            alb.ingress.kubernetes.io/actions.response-503: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"503","messageBody":"503 error text"}}
            alb.ingress.kubernetes.io/actions.redirect-to-eks: >
              {"type":"redirect","redirectConfig":{"host":"aws.amazon.com","path":"/eks/","port":"443","protocol":"HTTPS","query":"k=v","statusCode":"HTTP_302"}}
            alb.ingress.kubernetes.io/actions.forward-single-tg: >
              {"type":"forward","targetGroupARN": "arn-of-your-target-group"}
            alb.ingress.kubernetes.io/actions.forward-multiple-tg: >
              {"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"service-1","servicePort":"http","weight":20},{"serviceName":"service-2","servicePort":80,"weight":20},{"targetGroupARN":"arn-of-your-non-k8s-target-group","weight":60}],"targetGroupStickinessConfig":{"enabled":true,"durationSeconds":200}}}
        spec:
          ingressClassName: alb
          rules:
            - http:
                paths:
                  - path: /503
                    pathType: Exact
                    backend:
                      service:
                        name: response-503
                        port:
                          name: use-annotation
                  - path: /eks
                    pathType: Exact
                    backend:
                      service:
                        name: redirect-to-eks
                        port:
                          name: use-annotation
                  - path: /path1
                    pathType: Exact
                    backend:
                      service:
                        name: forward-single-tg
                        port:
                          name: use-annotation
                  - path: /path2
                    pathType: Exact
                    backend:
                      service:
                        name: forward-multiple-tg
                        port:
                          name: use-annotation
        ```

- <a name="conditions">`alb.ingress.kubernetes.io/conditions.${conditions-name}`</a> Provides a method for specifying routing conditions **in addition to original host/path condition on Ingress spec**.

    The `conditions-name` in the annotation must match the serviceName in the Ingress rules.
    It can be a either real serviceName or an annotation based action name when servicePort is `use-annotation`.

    !!!warning "limitations"
        General ALB limitations applies:

        1. Each rule can optionally include up to one of each of the following conditions: host-header, http-request-method, path-pattern, and source-ip. Each rule can also optionally include one or more of each of the following conditions: http-header and query-string.

        2. You can specify up to three match evaluations per condition.

        3. You can specify up to five match evaluations per rule.

        Refer [ALB documentation](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html#rule-condition-types) for more details.

    !!!example
        - rule-path1:
            - Host is www.example.com OR anno.example.com
            - Path is /path1
        - rule-path2:
            - Host is www.example.com
            - Path is /path2 OR /anno/path2
        - rule-path3:
            - Host is www.example.com
            - Path is /path3
            - Http header HeaderName is HeaderValue1 OR HeaderValue2
        - rule-path4:
            - Host is www.example.com
            - Path is /path4
            - Http request method is GET OR HEAD
        - rule-path5:
            - Host is www.example.com
            - Path is /path5
            - Query string is paramA:valueA1 OR paramA:valueA2
        - rule-path6:
            - Host is www.example.com
            - Path is /path6
            - Source IP is192.168.0.0/16 OR 172.16.0.0/16
        - rule-path7:
            - Host is www.example.com
            - Path is /path7
            - Http header HeaderName is HeaderValue
            - Query string is paramA:valueA
            - Query string is paramB:valueB

        ```yaml
        apiVersion: networking.k8s.io/v1
        kind: Ingress
        metadata:
          namespace: default
          name: ingress
          annotations:
            alb.ingress.kubernetes.io/scheme: internet-facing
            alb.ingress.kubernetes.io/actions.rule-path1: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"Host is www.example.com OR anno.example.com"}}
            alb.ingress.kubernetes.io/conditions.rule-path1: >
              [{"field":"host-header","hostHeaderConfig":{"values":["anno.example.com"]}}]
            alb.ingress.kubernetes.io/actions.rule-path2: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"Path is /path2 OR /anno/path2"}}
            alb.ingress.kubernetes.io/conditions.rule-path2: >
              [{"field":"path-pattern","pathPatternConfig":{"values":["/anno/path2"]}}]
            alb.ingress.kubernetes.io/actions.rule-path3: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"Http header HeaderName is HeaderValue1 OR HeaderValue2"}}
            alb.ingress.kubernetes.io/conditions.rule-path3: >
              [{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue1", "HeaderValue2"]}}]
            alb.ingress.kubernetes.io/actions.rule-path4: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"Http request method is GET OR HEAD"}}
            alb.ingress.kubernetes.io/conditions.rule-path4: >
              [{"field":"http-request-method","httpRequestMethodConfig":{"Values":["GET", "HEAD"]}}]
            alb.ingress.kubernetes.io/actions.rule-path5: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"Query string is paramA:valueA1 OR paramA:valueA2"}}
            alb.ingress.kubernetes.io/conditions.rule-path5: >
              [{"field":"query-string","queryStringConfig":{"values":[{"key":"paramA","value":"valueA1"},{"key":"paramA","value":"valueA2"}]}}]
            alb.ingress.kubernetes.io/actions.rule-path6: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"Source IP is 192.168.0.0/16 OR 172.16.0.0/16"}}
            alb.ingress.kubernetes.io/conditions.rule-path6: >
              [{"field":"source-ip","sourceIpConfig":{"values":["192.168.0.0/16", "172.16.0.0/16"]}}]
            alb.ingress.kubernetes.io/actions.rule-path7: >
              {"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"multiple conditions applies"}}
            alb.ingress.kubernetes.io/conditions.rule-path7: >
              [{"field":"http-header","httpHeaderConfig":{"httpHeaderName": "HeaderName", "values":["HeaderValue"]}},{"field":"query-string","queryStringConfig":{"values":[{"key":"paramA","value":"valueA"}]}},{"field":"query-string","queryStringConfig":{"values":[{"key":"paramB","value":"valueB"}]}}]
        spec:
          ingressClassName: alb
          rules:
            - host: www.example.com
              http:
                paths:
                  - path: /path1
                    pathType: Exact
                    backend:
                      service:
                        name: rule-path1
                        port:
                          name: use-annotation
                  - path: /path2
                    pathType: Exact
                    backend:
                      service:
                        name: rule-path2
                        port:
                          name: use-annotation
                  - path: /path3
                    pathType: Exact
                    backend:
                      service:
                        name: rule-path3
                        port:
                          name: use-annotation
                  - path: /path4
                    pathType: Exact
                    backend:
                      service:
                        name: rule-path4
                        port:
                          name: use-annotation
                  - path: /path5
                    pathType: Exact
                    backend:
                      service:
                        name: rule-path5
                        port:
                          name: use-annotation
                  - path: /path6
                    pathType: Exact
                    backend:
                      service:
                        name: rule-path6
                        port:
                          name: use-annotation
                  - path: /path7
                    pathType: Exact
                    backend:
                      service:
                        name: rule-path7
                        port:
                          name: use-annotation
        ```

    !!!note 
        If you are using `alb.ingress.kubernetes.io/target-group-attributes` with `stickiness.enabled=true`, you should add `TargetGroupStickinessConfig` under `alb.ingress.kubernetes.io/actions.weighted-routing`
        
    !!!example

        ```yaml
            apiVersion: networking.k8s.io/v1
            kind: Ingress
            metadata:
            namespace: default
            name: ingress
            annotations:
                alb.ingress.kubernetes.io/scheme: internet-facing
                alb.ingress.kubernetes.io/target-type: ip
                alb.ingress.kubernetes.io/target-group-attributes: stickiness.enabled=true,stickiness.lb_cookie.duration_seconds=60
                alb.ingress.kubernetes.io/actions.weighted-routing: |
                {
                    "type":"forward",
                    "forwardConfig":{
                    "targetGroups":[
                        {
                        "serviceName":"service-1",
                        "servicePort":"80",
                        "weight":50
                        },
                        {
                        "serviceName":"service-2",
                        "servicePort":"80",
                        "weight":50
                        }
                    ],
                    "TargetGroupStickinessConfig": {
                        "Enabled": true,
                        "DurationSeconds": 120
                    }
                    }
                }
            spec:
            ingressClassName: alb
            rules:
                - host: www.example.com
                http:
                    paths:
                    - path: /
                        pathType: Prefix
                        backend:
                        service:
                            name: weighted-routing
                            port:
                            name: use-annotation
        ```

## Access control
Access control for LoadBalancer can be controlled with following annotations:

- <a name="scheme">`alb.ingress.kubernetes.io/scheme`</a> specifies whether your LoadBalancer will be internet facing. See [Load balancer scheme](http://docs.aws.amazon.com/elasticloadbalancing/latest/userguide/how-elastic-load-balancing-works.html#load-balancer-scheme) in the AWS documentation for more details.

    !!!example
        ```
        alb.ingress.kubernetes.io/scheme: internal
        ```

- <a name="inbound-cidrs">`alb.ingress.kubernetes.io/inbound-cidrs`</a> specifies the CIDRs that are allowed to access LoadBalancer.

    !!!note "Merge Behavior"
        `inbound-cidrs` is merged across all Ingresses in IngressGroup, but is exclusive per listen-port.

        - the `inbound-cidrs` will only impact the ports defined for that Ingress.
        - if same listen-port is defined by multiple Ingress within IngressGroup, `inbound-cidrs` should only be defined on one of the Ingress.

    !!!note "Default"

        - `0.0.0.0/0` will be used if the IPAddressType is "ipv4"
        - `0.0.0.0/0` and `::/0` will be used if the IPAddressType is "dualstack"

    !!!warning ""
        this annotation will be ignored if `alb.ingress.kubernetes.io/security-groups` is specified.

    !!!example
        ```
        alb.ingress.kubernetes.io/inbound-cidrs: 10.0.0.0/24
        ```

- <a name="security-group-prefix-lists">`alb.ingress.kubernetes.io/security-group-prefix-lists`</a> specifies the managed prefix lists that are allowed to access LoadBalancer.

    !!!note "Merge Behavior"
        `security-group-prefix-lists` is merged across all Ingresses in IngressGroup, but is exclusive per listen-port.

        - the `security-group-prefix-lists` will only impact the ports defined for that Ingress.
        - if same listen-port is defined by multiple Ingress within IngressGroup, `security-group-prefix-lists` should only be defined on one of the Ingress.

    !!!warning ""
        This annotation will be ignored if `alb.ingress.kubernetes.io/security-groups` is specified.

    !!!warning ""
        If you'd like to use this annotation, make sure your security group rule quota is enough. If you'd like to know how the managed prefix list affects your quota, see the [reference](https://docs.aws.amazon.com/vpc/latest/userguide/working-with-aws-managed-prefix-lists.html#aws-managed-prefix-list-weights) in the AWS documentation for more details.

    !!!tip ""
        If you only use this annotation without `inbound-cidrs`, the controller managed security group would ignore the `inbound-cidrs` default settings.

    !!!example
        ```
        alb.ingress.kubernetes.io/security-group-prefix-lists: pl-000000, pl-111111
        ```

- <a name="security-groups">`alb.ingress.kubernetes.io/security-groups`</a> specifies the securityGroups you want to attach to LoadBalancer.

    !!!note ""
        When this annotation is not present, the controller will automatically create one security group, the security group will be attached to the LoadBalancer and allow access from [`inbound-cidrs`](#inbound-cidrs) and [`security-group-prefix-lists`](#security-group-prefix-lists) to the [`listen-ports`](#listen-ports).
        Also, the securityGroups for Node/Pod will be modified to allow inbound traffic from this securityGroup.

    !!!note ""
        If you specify this annotation, you need to configure the security groups on your Node/Pod to allow inbound traffic from the load balancer. You could also set the [`manage-backend-security-group-rules`](#manage-backend-security-group-rules) if you want the controller to manage the access rules.

    !!!tip ""
        Both name or ID of securityGroups are supported. Name matches a `Name` tag, not the `groupName` attribute.

    !!!example
        ```
        alb.ingress.kubernetes.io/security-groups: sg-xxxx, nameOfSg1, nameOfSg2
        ```

- <a name="manage-backend-security-group-rules">`alb.ingress.kubernetes.io/manage-backend-security-group-rules`</a> specifies whether you want the controller to configure security group rules on Node/Pod for traffic access when you specify [`security-groups`](#security-groups).

    !!!note ""
        This annotation applies only in case you specify the security groups via [`security-groups`](#security-groups) annotation. If set to true, controller attaches an additional shared backend security group to your load balancer. This backend security group is used in the Node/Pod security group rules.

    !!!example
        ```
        alb.ingress.kubernetes.io/manage-backend-security-group-rules: "true"
        ```

## Authentication
ALB supports authentication with Cognito or OIDC. See [Authenticate Users Using an Application Load Balancer](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html) for more details.

!!!warning "HTTPS only"
    Authentication is only supported for HTTPS listeners. See [TLS](#tls) for configuring HTTPS listeners.

- <a name="auth-type">`alb.ingress.kubernetes.io/auth-type`</a> specifies the authentication type on targets.

    !!!example
        ```
        alb.ingress.kubernetes.io/auth-type: cognito
        ```

- <a name="auth-idp-cognito">`alb.ingress.kubernetes.io/auth-idp-cognito`</a> specifies the cognito idp configuration.

    !!!tip ""
        If you are using Amazon Cognito Domain, the `userPoolDomain` should be set to the domain prefix(my-domain) instead of full domain(https://my-domain.auth.us-west-2.amazoncognito.com)

    !!!example
        ```
        alb.ingress.kubernetes.io/auth-idp-cognito: '{"userPoolARN":"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx","userPoolClientID":"my-clientID","userPoolDomain":"my-domain"}'
        ```

- <a name="auth-idp-oidc">`alb.ingress.kubernetes.io/auth-idp-oidc`</a> specifies the oidc idp configuration.

    !!!tip ""
        You need to create an [secret](https://kubernetes.io/docs/concepts/configuration/secret/) within the same namespace as Ingress to hold your OIDC clientID and clientSecret. The format of secret is as below:
        ```yaml
        apiVersion: v1
        kind: Secret
        metadata:
          namespace: testcase
          name: my-k8s-secret
        data:
          clientID: base64 of your plain text clientId
          clientSecret: base64 of your plain text clientSecret
        ```

    !!!example
        ```
        alb.ingress.kubernetes.io/auth-idp-oidc: '{"issuer":"https://example.com","authorizationEndpoint":"https://authorization.example.com","tokenEndpoint":"https://token.example.com","userInfoEndpoint":"https://userinfo.example.com","secretName":"my-k8s-secret"}'
        ```

- <a name="auth-on-unauthenticated-request">`alb.ingress.kubernetes.io/auth-on-unauthenticated-request`</a> specifies the behavior if the user is not authenticated.

	!!!info "options:"
        * **authenticate**: try authenticate with configured IDP.
        * **deny**: return an HTTP 401 Unauthorized error.
        * **allow**: allow the request to be forwarded to the target.

    !!!example
        ```
        alb.ingress.kubernetes.io/auth-on-unauthenticated-request: authenticate
        ```

- <a name="auth-scope">`alb.ingress.kubernetes.io/auth-scope`</a> specifies the set of user claims to be requested from the IDP(cognito or oidc), in a space-separated list.

	!!!info "options:"
	    * **phone**
	    * **email**
	    * **profile**
	    * **openid**
	    * **aws.cognito.signin.user.admin**

    !!!example
        ```
        alb.ingress.kubernetes.io/auth-scope: 'email openid'
        ```

- <a name="auth-session-cookie">`alb.ingress.kubernetes.io/auth-session-cookie`</a> specifies the name of the cookie used to maintain session information

    !!!example
        ```
        alb.ingress.kubernetes.io/auth-session-cookie: custom-cookie
        ```

- <a name="auth-session-timeout">`alb.ingress.kubernetes.io/auth-session-timeout`</a> specifies the maximum duration of the authentication session, in seconds

    !!!example
        ```
        alb.ingress.kubernetes.io/auth-session-timeout: '86400'
        ```

## Health Check
Health check on target groups can be controlled with following annotations:

- <a name="healthcheck-protocol">`alb.ingress.kubernetes.io/healthcheck-protocol`</a> specifies the protocol used when performing health check on targets.

    !!!example
        ```
        alb.ingress.kubernetes.io/healthcheck-protocol: HTTPS
        ```

- <a name="healthcheck-port">`alb.ingress.kubernetes.io/healthcheck-port`</a> specifies the port used when performing health check on targets.

    !!!warning ""
        When using `target-type: instance` with a service of type "NodePort", the healthcheck port can be set to `traffic-port` to automatically point to the correct port.

    !!!example
        - set the healthcheck port to the traffic port
            ```
            alb.ingress.kubernetes.io/healthcheck-port: traffic-port
            ```
        - set the healthcheck port to the NodePort(when target-type=instance) or TargetPort(when target-type=ip) of a named port
            ```
            alb.ingress.kubernetes.io/healthcheck-port: my-port
            ```
        - set the healthcheck port to 80/tcp
            ```
            alb.ingress.kubernetes.io/healthcheck-port: '80'
            ```

- <a name="healthcheck-path">`alb.ingress.kubernetes.io/healthcheck-path`</a> specifies the HTTP path when performing health check on targets.

    !!!example
        - HTTP
            ```
            alb.ingress.kubernetes.io/healthcheck-path: /ping
            ```
        - GRPC
            ```
            alb.ingress.kubernetes.io/healthcheck-path: /package.service/method
            ```

- <a name="healthcheck-interval-seconds">`alb.ingress.kubernetes.io/healthcheck-interval-seconds`</a> specifies the interval(in seconds) between health check of an individual target.

    !!!example
        ```
        alb.ingress.kubernetes.io/healthcheck-interval-seconds: '10'
        ```

- <a name="healthcheck-timeout-seconds">`alb.ingress.kubernetes.io/healthcheck-timeout-seconds`</a> specifies the timeout(in seconds) during which no response from a target means a failed health check

    !!!example
        ```
        alb.ingress.kubernetes.io/healthcheck-timeout-seconds: '8'
        ```

- <a name="success-codes">`alb.ingress.kubernetes.io/success-codes`</a> specifies the HTTP or gRPC status code that should be expected when doing health checks against the specified health check path.

    !!!example
        - use single value
            ```
            alb.ingress.kubernetes.io/success-codes: '200'
            ```
        - use multiple values
            ```
            alb.ingress.kubernetes.io/success-codes: 200,201
            ```
        - use range of value
            ```
            alb.ingress.kubernetes.io/success-codes: 200-300
            ```
        - use gRPC single value
            ```
            alb.ingress.kubernetes.io/success-codes: '0'
            ```
        - use gRPC multiple value
            ```
            alb.ingress.kubernetes.io/success-codes: 0,1
            ```
        - use gRPC range of value
            ```
            alb.ingress.kubernetes.io/success-codes: 0-5
            ```

- <a name="healthy-threshold-count">`alb.ingress.kubernetes.io/healthy-threshold-count`</a> specifies the consecutive health checks successes required before considering an unhealthy target healthy.

    !!!example
        ```
        alb.ingress.kubernetes.io/healthy-threshold-count: '2'
        ```

- <a name="unhealthy-threshold-count">`alb.ingress.kubernetes.io/unhealthy-threshold-count`</a> specifies the consecutive health check failures required before considering a target unhealthy.

    !!!example
        ```alb.ingress.kubernetes.io/unhealthy-threshold-count: '2'
        ```

## TLS
TLS support can be controlled with the following annotations:

- <a name="certificate-arn">`alb.ingress.kubernetes.io/certificate-arn`</a> specifies the ARN of one or more certificate managed by [AWS Certificate Manager](https://aws.amazon.com/certificate-manager)

    !!!tip ""
        The first certificate in the list will be added as default certificate. And remaining certificate will be added to the optional certificate list.
        See [SSL Certificates](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/create-https-listener.html#https-listener-certificates) for more details.

    !!!tip "Certificate Discovery"
        TLS certificates for ALB Listeners can be automatically discovered with hostnames from Ingress resources. See [Certificate Discovery](cert_discovery.md) for instructions.

    !!!example
        - single certificate
            ```
            alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-west-2:xxxxx:certificate/xxxxxxx
            ```
        - multiple certificates
            ```
            alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-west-2:xxxxx:certificate/cert1,arn:aws:acm:us-west-2:xxxxx:certificate/cert2,arn:aws:acm:us-west-2:xxxxx:certificate/cert3
            ```

- <a name="ssl-policy">`alb.ingress.kubernetes.io/ssl-policy`</a> specifies the [Security Policy](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/create-https-listener.html#describe-ssl-policies) that should be assigned to the ALB, allowing you to control the protocol and ciphers.

    !!!example
        ```
        alb.ingress.kubernetes.io/ssl-policy: ELBSecurityPolicy-TLS-1-1-2017-01
        ```

- <a name="mutual-authentication">`alb.ingress.kubernetes.io/mutual-authentication`</a>  specifies the mutual authentication configuration that should be assigned to the Application Load Balancer secure listener ports. See [Mutual authentication with TLS](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/mutual-authentication.html) in the AWS documentation for more details.

    !!!note 
        - This annotation is not applicable for Outposts, Local Zones or Wavelength zones.
        - "Configuration Options"
            - `port: listen port ` 
               - Must be a HTTPS port specified by [listen-ports](#listen-ports).
            - `mode: "off" (default) | "passthrough" | "verify"`
               - `verify` mode requires an existing trust store resource.
               -  See [Create a trust store](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/mutual-authentication.html#create-trust-store) in the AWS documentation for more details.
            - `trustStore: ARN (arn:aws:elasticloadbalancing:trustStoreArn) | Name (my-trust-store)`
               - Both ARN and Name of trustStore are supported values.
               - `trustStore` is required when mode is `verify`.
            - `ignoreClientCertificateExpiry : true | false (default)`
        - Once the Mutual Authentication is set, to turn it off, you will have to explicitly pass in this annotation with `mode : "off"`.
  
    !!!example
        - [listen-ports](#listen-ports) specifies four HTTPS ports: `80, 443, 8080, 8443`
        - listener `HTTPS:80` will be set to `passthrough` mode
        - listener `HTTPS:443` will be set to `verify` mode, associated with trust store arn `arn:aws:elasticloadbalancing:trustStoreArn` and have `ignoreClientCertificateExpiry` set to `true`
        - listeners `HTTPS:8080` and `HTTPS:8443` remain in the default mode `off`.
            ```
            alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS": 80}, {"HTTPS": 443}, {"HTTPS": 8080}, {"HTTPS": 8443}]'
            alb.ingress.kubernetes.io/mutual-authentication: '[{"port": 80, "mode": "passthrough"},
                                                               {"port": 443, "mode": "verify", "trustStore": "arn:aws:elasticloadbalancing:trustStoreArn", "ignoreClientCertificateExpiry" : true}]'
            ```

    !!!note "Note"
        To avoid conflict errors in IngressGroup, this annotation should only be specified on a single Ingress within IngressGroup or specified with same value across all Ingresses within IngressGroup.

    !!!warning "Trust stores limit per Application Load Balancer"
        A maximum of two different trust stores can be associated among listeners on the same ingress. See [Quotas for your Application Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-limits.html) in the AWS documentation for more details.

## Custom attributes
Custom attributes to LoadBalancers and TargetGroups can be controlled with following annotations:

- <a name="load-balancer-attributes">`alb.ingress.kubernetes.io/load-balancer-attributes`</a> specifies [Load Balancer Attributes](http://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_LoadBalancerAttribute.html) that should be applied to the ALB.

    !!!warning ""
        Only attributes defined in the annotation will be updated. To unset any AWS defaults(e.g. Disabling access logs after having them enabled once), the values need to be explicitly set to the original values(`access_logs.s3.enabled=false`) and omitting them is not sufficient.

    !!!note ""
        - If `deletion_protection.enabled=true` is in annotation, the controller will not be able to delete the ALB during reconciliation. Once the attribute gets edited to `deletion_protection.enabled=false` during reconciliation, the deployer will force delete the resource.
        - Please note, if the deletion protection is not enabled via annotation (e.g. via AWS console), the controller still deletes the underlying resource.

    !!!example
        - enable access log to s3
            ```
            alb.ingress.kubernetes.io/load-balancer-attributes: access_logs.s3.enabled=true,access_logs.s3.bucket=my-access-log-bucket,access_logs.s3.prefix=my-app
            ```
        - enable deletion protection
            ```
            alb.ingress.kubernetes.io/load-balancer-attributes: deletion_protection.enabled=true
            ```
        - enable invalid header fields removal
            ```
            alb.ingress.kubernetes.io/load-balancer-attributes: routing.http.drop_invalid_header_fields.enabled=true
            ```
        - enable http2 support
            ```
            alb.ingress.kubernetes.io/load-balancer-attributes: routing.http2.enabled=true
            ```
        - set idle_timeout delay to 600 seconds
            ```
            alb.ingress.kubernetes.io/load-balancer-attributes: idle_timeout.timeout_seconds=600
            ```
        - set client_keep_alive to 3600 seconds
            ```
            alb.ingress.kubernetes.io/load-balancer-attributes: client_keep_alive.seconds=3600  
            ```
        - enable [connection logs](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-connection-logs.html)
            ```
            alb.ingress.kubernetes.io/load-balancer-attributes: connection_logs.s3.enabled=true,connection_logs.s3.bucket=my-connection-log-bucket,connection_logs.s3.prefix=my-app
            ```
- <a name="target-group-attributes">`alb.ingress.kubernetes.io/target-group-attributes`</a> specifies [Target Group Attributes](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html#target-group-attributes) which should be applied to Target Groups.

    !!!example
        - set the slow start duration to 30 seconds (available range is 30-900 seconds)
            ```
            alb.ingress.kubernetes.io/target-group-attributes: slow_start.duration_seconds=30
            ```
        - set the deregistration delay to 30 seconds (available range is 0-3600 seconds)
            ```
            alb.ingress.kubernetes.io/target-group-attributes: deregistration_delay.timeout_seconds=30
            ```
        - enable sticky sessions (requires `alb.ingress.kubernetes.io/target-type` be set to `ip`)
            ```
            alb.ingress.kubernetes.io/target-group-attributes: stickiness.enabled=true,stickiness.lb_cookie.duration_seconds=60
            alb.ingress.kubernetes.io/target-type: ip
            ```
        - set load balancing algorithm to least outstanding requests
                    ```
                    alb.ingress.kubernetes.io/target-group-attributes: load_balancing.algorithm.type=least_outstanding_requests
                    ```
        - enable Automated Target Weights(ATW) on HTTP/HTTPS target groups to increase application availability. Set your load balancing algorithm to weighted random and turn on anomaly mitigation (recommended)
            ```
            alb.ingress.kubernetes.io/target-group-attributes: load_balancing.algorithm.type=weighted_random,load_balancing.algorithm.anomaly_mitigation=on
            ```

## Resource Tags
The AWS Load Balancer Controller automatically applies following tags to the AWS resources (ALB/TargetGroups/SecurityGroups/Listener/ListenerRule) it creates:

- `elbv2.k8s.aws/cluster: ${clusterName}`
- `ingress.k8s.aws/stack: ${stackID}`
- `ingress.k8s.aws/resource: ${resourceID}`

In addition, you can use annotations to specify additional tags

- <a name="tags">`alb.ingress.kubernetes.io/tags`</a> specifies additional tags that will be applied to AWS resources created.
  In case of target group, the controller will merge the tags from the ingress and the backend service giving precedence
  to the values specified on the service when there is conflict.

    !!!example
        ```
        alb.ingress.kubernetes.io/tags: Environment=dev,Team=test
        ```

## Addons

- <a name="waf-acl-id">`alb.ingress.kubernetes.io/waf-acl-id`</a> specifies the identifier for the Amazon WAF Classic web ACL.

    !!!warning ""
        Only Regional WAF Classic is supported.

    !!!note ""
        When this annotation is absent or empty, the controller will keep LoadBalancer WAF Classic settings unchanged.
        To disable WAF Classic, explicitly set the annotation value to 'none'.

    !!!example
        - enable WAF Classic
            ```alb.ingress.kubernetes.io/waf-acl-id: 499e8b99-6671-4614-a86d-adb1810b7fbe
            ```
        - disable WAF Classic
            ```alb.ingress.kubernetes.io/waf-acl-id: none
            ```

- <a name="wafv2-acl-arn">`alb.ingress.kubernetes.io/wafv2-acl-arn`</a> specifies ARN for the Amazon WAFv2 web ACL.

    !!!warning ""
        Only Regional WAFv2 is supported.

    !!!note ""
        When this annotation is absent or empty, the controller will keep LoadBalancer WAFv2 settings unchanged.
        To disable WAFv2, explicitly set the annotation value to 'none'.

    !!!tip ""
        To get the WAFv2 Web ACL ARN from the Console, click the gear icon in the upper right and enable the ARN column.

    !!!example
        - enable WAFv2
            ```alb.ingress.kubernetes.io/wafv2-acl-arn: arn:aws:wafv2:us-west-2:xxxxx:regional/webacl/xxxxxxx/3ab78708-85b0-49d3-b4e1-7a9615a6613b
            ```
        - disable WAFV2
            ```alb.ingress.kubernetes.io/wafv2-acl-arn: none
            ```
  
- <a name="shield-advanced-protection">`alb.ingress.kubernetes.io/shield-advanced-protection`</a> turns on / off the AWS Shield Advanced protection for the load balancer.

    !!!note ""
        When this annotation is absent, the controller will keep LoadBalancer shield protection settings unchanged.
        To disable shield protection, explicitly set the annotation value to 'false'.

    !!!example
        - enable shield protection
            ```alb.ingress.kubernetes.io/shield-advanced-protection: 'true'
            ```
        - disable shield protection
            ```alb.ingress.kubernetes.io/shield-advanced-protection: 'false'
            ```
