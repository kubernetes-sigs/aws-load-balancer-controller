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
|Name                       | Type |Default|Location|MergeBehavior|
|---------------------------|------|-------|--------|------|
|[alb.ingress.kubernetes.io/group.name](#group.name)|string|N/A|Ingress|N/A|
|[alb.ingress.kubernetes.io/group.order](#group.order)|integer|0|Ingress|N/A|
|[alb.ingress.kubernetes.io/tags](#tags)|stringMap|N/A|Ingress,Service|Merge|
|[alb.ingress.kubernetes.io/ip-address-type](#ip-address-type)|ipv4 \| dualstack|ipv4|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/scheme](#scheme)|internal \| internet-facing|internal|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/subnets](#subnets)|stringList|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/security-groups](#security-groups)|stringList|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/customer-owned-ipv4-pool](#customer-owned-ipv4-pool)|string|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/load-balancer-attributes](#load-balancer-attributes)|stringMap|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/wafv2-acl-arn](#wafv2-acl-arn)|string|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/waf-acl-id](#waf-acl-id)|string|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/shield-advanced-protection](#shield-advanced-protection)|boolean|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/listen-ports](#listen-ports)|json|'[{"HTTP": 80}]' \| '[{"HTTPS": 443}]'|Ingress|Merge|
|[alb.ingress.kubernetes.io/ssl-redirect](#ssl-redirect)|integer|N/A|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/inbound-cidrs](#inbound-cidrs)|stringList|0.0.0.0/0, ::/0|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/certificate-arn](#certificate-arn)|stringList|N/A|Ingress|Merge|
|[alb.ingress.kubernetes.io/ssl-policy](#ssl-policy)|string|ELBSecurityPolicy-2016-08|Ingress|Exclusive|
|[alb.ingress.kubernetes.io/target-type](#target-type)|instance \| ip|instance|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/backend-protocol](#backend-protocol)|HTTP \| HTTPS|HTTP|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/backend-protocol-version](#backend-protocol-version)|string | HTTP1 |Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/target-group-attributes](#target-group-attributes)|stringMap|N/A|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/healthcheck-port](#healthcheck-port)|integer \| traffic-port|traffic-port|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/healthcheck-protocol](#healthcheck-protocol)|HTTP \| HTTPS|HTTP|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/healthcheck-path](#healthcheck-path)|string|/ \| /AWS.ALB/healthcheck |Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/healthcheck-interval-seconds](#healthcheck-interval-seconds)|integer|'15'|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/healthcheck-timeout-seconds](#healthcheck-timeout-seconds)|integer|'5'|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/healthy-threshold-count](#healthy-threshold-count)|integer|'2'|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/unhealthy-threshold-count](#unhealthy-threshold-count)|integer|'2'|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/success-codes](#success-codes)|string|'200' \| '12' |Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/auth-type](#auth-type)|none\|oidc\|cognito|none|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/auth-idp-cognito](#auth-idp-cognito)|json|N/A|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/auth-idp-oidc](#auth-idp-oidc)|json|N/A|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/auth-on-unauthenticated-request](#auth-on-unauthenticated-request)|authenticate\|allow\|deny|authenticate|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/auth-scope](#auth-scope)|string|openid|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/auth-session-cookie](#auth-session-cookie)|string|AWSELBAuthSessionCookie|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/auth-session-timeout](#auth-session-timeout)|integer|'604800'|Ingress,Service|N/A|
|[alb.ingress.kubernetes.io/actions.${action-name}](#actions)|json|N/A|Ingress|N/A|
|[alb.ingress.kubernetes.io/conditions.${conditions-name}](#conditions)|json|N/A|Ingress|N/A|

## IngressGroup
IngressGroup feature enables you to group multiple Ingress resources together.
The controller will automatically merge Ingress rules for all Ingresses within IngressGroup and support them with a single ALB.
In addition, most annotations defined on a Ingress only applies to the paths defined by that Ingress.

By default, Ingresses don't belong to any IngressGroup, and we treat it as a "implicit IngressGroup" consisted of the Ingress itself.

- <a name="group.name">`alb.ingress.kubernetes.io/group.name`</a> specifies the group name that this Ingress belongs to.

    !!!note ""
        - Ingresses with same `group.name` annotation will form as a "explicit IngressGroup".
        - groupName must consist of lower case alphanumeric characters, `-` or `.`, and must start and end with an alphanumeric character.
        - groupName must be no more than 63 character.

    !!!warning "Security Risk"
        IngressGroup feature should only be used when all Kubernetes users with RBAC permission to create/modify Ingress resources are within trust boundary.
        
        If you turn your Ingress to belong a "explicit IngressGroup" by adding `group.name` annotation,
        other Kubernetes user may create/modify their Ingresses to belong same IngressGroup, thus can add more rules or overwrite existing rules with higher priority to the ALB for your Ingress.

        We'll add more fine-grained access-control in future versions.

    !!!example
        ```
        alb.ingress.kubernetes.io/group.name: my-team.awesome-group
        ```

- <a name="group.order">`alb.ingress.kubernetes.io/group.order`</a> specifies the order across all Ingresses within IngressGroup.
    
    !!!note ""
        - You can explicitly denote the order using a number between 1-1000
        - The smaller the order, the rule will be evaluated first. All Ingresses without explicit order setting get order value as 0
        - By default the rule order between Ingresses within IngressGroup are determined by the lexical order of Ingress’s namespace/name.

    !!!warning "" 
        You may not have duplicate group order explicitly defined for Ingresses within IngressGroup.

    !!!example
        ```
        alb.ingress.kubernetes.io/group.order: '10'
        ```

## Traffic Listening
Traffic Listening can be controlled with following annotations:

- <a name="listen-ports">`alb.ingress.kubernetes.io/listen-ports`</a> specifies the ports that ALB used to listen on.
    
    !!!note "Merge Behavior"
        `listen-ports` is merged across all Ingresses in IngressGroup.
            
        - You can define different listen-ports per Ingress, Ingress rules will only impact the ports defined for that Ingress.
        - If same listen-port is defined by multiple Ingress within IngressGroup, Ingress rules will be merged with respect to their group order within IngressGroup.

    !!!note "Default"
        - defaults to `'[{"HTTP": 80}]'` or `'[{"HTTPS": 443}]'` depends on whether `certificate-arn` is specified.

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
        - Once enabled SSLRedirect, every HTTP listener will be configured with default action which redirects to HTTPS, other rules will be ignored.
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

- <a name="target-type">`alb.ingress.kubernetes.io/target-type`</a> specifies how to route traffic to pods. You can choose between `instance` and `ip`:

    - `instance` mode will route traffic to all ec2 instances within cluster on [NodePort](https://kubernetes.io/docs/concepts/services-networking/service/#nodeport) opened for your service.

        !!!note ""
            service must be of type "NodePort" or "LoadBalancer" to use `instance` mode

    - `ip` mode will route traffic directly to the pod IP.

        !!!note ""
            network plugin must use secondary IP addresses on [ENI](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-eni.html) for pod IP to use `ip` mode. e.g.

            - [amazon-vpc-cni-k8s](https://github.com/aws/amazon-vpc-cni-k8s)

        !!!note ""
            `ip` mode is required for sticky sessions to work with Application Load Balancers.

    !!!example
        ```
        alb.ingress.kubernetes.io/target-type: instance
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

- <a name="subnets">`alb.ingress.kubernetes.io/subnets`</a> specifies the [Availability Zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html) that ALB will route traffic to. See [Load Balancer subnets](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-subnets.html) for more details.

    !!!note ""
        You must specify at least two subnets in different AZ. both subnetID or subnetName(Name tag on subnets) can be used.

    !!!tip
        You can enable subnet auto discovery to avoid specify this annotation on every Ingress. See [Subnet Discovery](../../deploy/subnet_discovery.md) for instructions.

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
        - forward-single-tg: forward to an single targetGroup [**simplified schema**]
        - forward-multiple-tg: forward to multiple targetGroups with different weights and stickiness config [**advanced schema**]

        ```yaml
        apiVersion: extensions/v1beta1
        kind: Ingress
        metadata:
          namespace: default
          name: ingress
          annotations:
            kubernetes.io/ingress.class: alb
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
          rules:
            - http:
                paths:
                  - path: /503
                    backend:
                      serviceName: response-503
                      servicePort: use-annotation
                  - path: /eks
                    backend:
                      serviceName: redirect-to-eks
                      servicePort: use-annotation
                  - path: /path1
                    backend:
                      serviceName: forward-single-tg
                      servicePort: use-annotation
                  - path: /path2
                    backend:
                      serviceName: forward-multiple-tg
                      servicePort: use-annotation
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
        apiVersion: extensions/v1beta1
        kind: Ingress
        metadata:
          namespace: default
          name: ingress
          annotations:
            kubernetes.io/ingress.class: alb
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
          rules:
            - host: www.example.com
              http:
                paths:
                  - path: /path1
                    backend:
                      serviceName: rule-path1
                      servicePort: use-annotation
                  - path: /path2
                    backend:
                      serviceName: rule-path2
                      servicePort: use-annotation
                  - path: /path3
                    backend:
                      serviceName: rule-path3
                      servicePort: use-annotation
                  - path: /path4
                    backend:
                      serviceName: rule-path4
                      servicePort: use-annotation
                  - path: /path5
                    backend:
                      serviceName: rule-path5
                      servicePort: use-annotation
                  - path: /path6
                    backend:
                      serviceName: rule-path6
                      servicePort: use-annotation
                  - path: /path7
                    backend:
                      serviceName: rule-path7
                      servicePort: use-annotation
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
        - if same listen-port is defined by multiple Ingress within IngressGroup, inbound-cidrs should only be defined on one of the Ingress.

    !!!note "Default"
        
        - `0.0.0.0/0` will be used if the IPAddressType is "ipv4"
        - `0.0.0.0/0` and `::/0` will be used if the IPAddressType is "dualstack"

    !!!warning ""
        this annotation will be ignored if `alb.ingress.kubernetes.io/security-groups` is specified.

    !!!example
        ```
        alb.ingress.kubernetes.io/inbound-cidrs: 10.0.0.0/24
        ```

- <a name="security-groups">`alb.ingress.kubernetes.io/security-groups`</a> specifies the securityGroups you want to attach to LoadBalancer.

    !!!note ""
        When this annotation is not present, the controller will automatically create one security groups: the security group will be attached to the LoadBalancer and allow access from [`inbound-cidrs`](#inbound-cidrs) to the [`listen-ports`](#listen-ports). 
        Also, the securityGroups for Node/Pod will be modified to allow inbound traffic from this securityGroup.

    !!!tip ""
        Both name or ID of securityGroups are supported. Name matches a `Name` tag, not the `groupName` attribute.

    !!!example
        ```
        alb.ingress.kubernetes.io/security-groups: sg-xxxx, nameOfSg1, nameOfSg2
        ```

## Authentication
ALB supports authentication with Cognito or OIDC. See [Authenticate Users Using an Application Load Balancer](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html) for more details.

!!!warning "HTTPS only"
    Authentication is only supported for HTTPS listeners, see [SSL](#ssl) for configure HTTPS listener.

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
        ```alb.ingress.kubernetes.io/healthcheck-protocol: HTTPS
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

- <a name="success-codes">`alb.ingress.kubernetes.io/success-codes`</a> specifies the HTTP status code that should be expected when doing health checks against the specified health check path.

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

- <a name="healthy-threshold-count">`alb.ingress.kubernetes.io/healthy-threshold-count`</a> specifies the consecutive health checks successes required before considering an unhealthy target healthy.

    !!!example
        ```
        alb.ingress.kubernetes.io/healthy-threshold-count: '2'
        ```

- <a name="unhealthy-threshold-count">`alb.ingress.kubernetes.io/unhealthy-threshold-count`</a> specifies the consecutive health check failures required before considering a target unhealthy.

    !!!example
        ```alb.ingress.kubernetes.io/unhealthy-threshold-count: '2'
        ```

## SSL
SSL support can be controlled with following annotations:

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

## Custom attributes
Custom attributes to LoadBalancers and TargetGroups can be controlled with following annotations:

- <a name="load-balancer-attributes">`alb.ingress.kubernetes.io/load-balancer-attributes`</a> specifies [Load Balancer Attributes](http://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_LoadBalancerAttribute.html) that should be applied to the ALB.

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

## Resource Tags
AWS Load Balancer Controller will automatically apply following tags to AWS resources(ALB/TargetGroups/SecurityGroups) created.

- `ingress.k8s.aws/cluster: ${clusterName}`
- `ingress.k8s.aws/stack: ${stackID}`
- `ingress.k8s.aws/resource: ${resourceID}`

In addition, you can use annotations to specify additional tags

- <a name="tags">`alb.ingress.kubernetes.io/tags`</a> specifies additional tags that will be applied to AWS resources created.

    !!!example
        ```
        alb.ingress.kubernetes.io/tags: Environment=dev,Team=test
        ```

## Addons
- <a name="waf-acl-id">`alb.ingress.kubernetes.io/waf-acl-id`</a> specifies the identifier for the Amzon WAF web ACL.

    !!!warning ""
        Only Regional WAF is supported.

    !!!example
        ```alb.ingress.kubernetes.io/waf-acl-id: 499e8b99-6671-4614-a86d-adb1810b7fbe
        ```

- <a name="wafv2-acl-arn">`alb.ingress.kubernetes.io/wafv2-acl-arn`</a> specifies ARN for the Amazon WAFv2 web ACL.

    !!!warning ""
        Only Regional WAFv2 is supported.

    !!!tip ""
        To get the WAFv2 Web ACL ARN from the Console, click the gear icon in the upper right and enable the ARN column.

    !!!example
        ```alb.ingress.kubernetes.io/wafv2-acl-arn: arn:aws:wafv2:us-west-2:xxxxx:regional/webacl/xxxxxxx/3ab78708-85b0-49d3-b4e1-7a9615a6613b
        ```

- <a name="shield-advanced-protection">`alb.ingress.kubernetes.io/shield-advanced-protection`</a> turns on / off the AWS Shield Advanced protection for the load balancer.

    !!!example
        ```alb.ingress.kubernetes.io/shield-advanced-protection: 'true'
        ```
