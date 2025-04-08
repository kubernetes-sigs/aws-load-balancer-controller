# [IngressClass](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-class)

Ingresses can be implemented by different controllers, often with different configuration. Each Ingress should specify a
class, a reference to an IngressClass resource that contains additional configuration including the name of the
controller that should implement the class. IngressClass resources contain an optional parameters field. This can be
used to reference additional implementation-specific configuration for this class.
For the AWS Load Balancer controller, the implementation-specific configuration is
[IngressClassParams](#ingressclassparams) in the `elbv2.k8s.aws` API group.

!!!example
    - specify controller as `ingress.k8s.aws/alb` to denote Ingresses should be managed by AWS Load Balancer Controller.
    ```
    apiVersion: networking.k8s.io/v1
    kind: IngressClass
    metadata:
      name: awesome-class
    spec:
      controller: ingress.k8s.aws/alb
    ```
    - specify additional configurations by referencing an IngressClassParams resource.
    ```
    apiVersion: networking.k8s.io/v1
    kind: IngressClass
    metadata:
      name: awesome-class
    spec:
      controller: ingress.k8s.aws/alb
      parameters:
        apiGroup: elbv2.k8s.aws
        kind: IngressClassParams
        name: awesome-class-cfg
    ```

!!!tip "[default IngressClass](https://kubernetes.io/docs/concepts/services-networking/ingress/#default-ingress-class)"
    You can mark a particular IngressClass as the default for your cluster. Setting the
    `ingressclass.kubernetes.io/is-default-class` annotation to `true` on an IngressClass resource will ensure that new
    Ingresses without an `ingressClassName` field specified will be assigned this default IngressClass.


## [Deprecated `kubernetes.io/ingress.class` annotation](https://kubernetes.io/docs/concepts/services-networking/ingress/#deprecated-annotation)

Before the IngressClass resource and `ingressClassName` field were added in Kubernetes 1.18, Ingress classes were
specified with a `kubernetes.io/ingress.class` annotation on the Ingress. This annotation was never formally defined,
but was widely supported by Ingress controllers.

The newer `ingressClassName` field on Ingresses is a replacement for that annotation, but is not a direct equivalent.
While the annotation was generally used to reference the name of the Ingress controller that should implement the
Ingress, the field is a reference to an IngressClass resource that contains additional Ingress configuration, including
the name of the Ingress controller.

!!!tip "disable `kubernetes.io/ingress.class` annotation"
    In order to maintain backwards-compatibility, `kubernetes.io/ingress.class` annotation is still supported currently.
    You can enforce IngressClass resource adoption by disabling the `kubernetes.io/ingress.class` annotation via [--disable-ingress-class-annotation](../../../deploy/configurations/#disable-ingress-class-annotation) controller flag.

## IngressClassParams
IngressClassParams is a [CRD](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) specific to the AWS Load Balancer Controller, which can be used along with IngressClass’s parameter field.
You can use IngressClassParams to enforce settings for a set of Ingresses.

!!!example
    - with scheme & ipAddressType & tags
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: awesome-class
    spec:
      scheme: internal
      ipAddressType: dualstack
      tags:
      - key: org
        value: my-org
    ```
    - with namespaceSelector
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: awesome-class
    spec:
      namespaceSelector:
        matchLabels:
          team: team-a
    ```
    - with IngressGroup
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: awesome-class
    spec:
      group:
        name: my-group
    ```
    - with loadBalancerAttributes
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
        name: awesome-class
    spec:
      loadBalancerAttributes:
      - key: deletion_protection.enabled
        value: "true"
      - key: idle_timeout.timeout_seconds
        value: "120"
    ```
    - with subnets.ids
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: awesome-class
    spec:
      subnets:
        ids:
        - subnet-xxx
        - subnet-123
    ```
    - with subnets.tags
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: class2048-config
    spec:
      subnets:
      tags:
        kubernetes.io/role/internal-elb:
        - "1"
        myKey:
        - myVal0
        - myVal1
    ```
    - with certificateArn
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
    name: class2048-config
    spec:
      certificateArn: ['arn:aws:acm:us-east-1:123456789:certificate/test-arn-1','arn:aws:acm:us-east-1:123456789:certificate/test-arn-2']
    ```
    - with minimumLoadBalancerCapacity.capacityUnits
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: class2048-config
    spec:
      minimumLoadBalancerCapacity:
        capacityUnits: 1000
    ``` 
    - with authenticationConfiguration type cognito
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: my-ingress-class-params
    spec:
      authenticationConfiguration:
        type: cognito
        idpCognitoConfiguration:
          userPoolARN: arn:aws:cognito-idp:us-east-x:xxxx  
          userPoolClientID: my-client-id                      
          userPoolDomain: us-east-1xxxx
        onUnauthenticatedRequest: deny
        sessionTimeout: 12345
    ```
    - with authenticationConfiguration type oidc
    ```
    apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: my-ingress-class-params
    spec:
      authenticationConfiguration:
      type: oidc
      idpOidcConfiguration:
        issuer: https://my-site.com
        authorizationEndpoint: https://super-strong-auth.my-site.com
        tokenEndpoint: https://token.my-site.com
        userInfoEndpoint: https://user.my-site.com
        secretName: top-secret
      authenticationRequestExtraParams:
        key: "value"
      onUnauthenticatedRequest: deny
      sessionTimeout: 12345
      scope: email openid

### IngressClassParams specification

#### spec.namespaceSelector

`namespaceSelector` is an optional setting that follows general Kubernetes
[label selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors)
semantics.

Cluster administrators can use the `namespaceSelector` field to restrict the namespaces of Ingresses that are allowed to specify the IngressClass.

1. If `namespaceSelector` specified, only Ingresses in selected namespaces can use IngressClasses with this parameter. The controller will refuse to reconcile for Ingresses that violates `namespaceSelector`.
2. If `namespaceSelector` un-specified, all Ingresses in any namespace can use IngressClasses with this parameter.

#### spec.group

`group` is an optional setting. The only available sub-field is `group.name`.

Cluster administrators can use `group.name` field to denote the groupName for all Ingresses belong to this IngressClass.

1. If `group.name` specified, all Ingresses with this IngressClass will belong to the same IngressGroup specified and result in a single ALB.
If `group.name` is not specified, Ingresses with this IngressClass can use the older / legacy `alb.ingress.kubernetes.io/group.name` annotation to specify their IngressGroup. Ingresses that belong to the same IngressClass can form different IngressGroups via that annotation.

#### spec.scheme

`scheme` is an optional setting. The available options are `internet-facing` or `internal`.

Cluster administrators can use the `scheme` field to restrict the scheme for all Ingresses that belong to this IngressClass.

1. If `scheme` specified, all Ingresses with this IngressClass will have the specified scheme.
2. If `scheme` un-specified, Ingresses with this IngressClass can continue to use `alb.ingress.kubernetes.io/scheme annotation` to specify scheme.

#### spec.inboundCIDRs

Cluster administrators can use the optional `inboundCIDRs` field to specify the CIDRs that are allowed to access the load balancers that belong to this IngressClass.
If the field is specified, LBC will ignore the `alb.ingress.kubernetes.io/inbound-cidrs` annotation.

#### spec.certificateArn

Cluster administrators can use the optional `certificateARN` field to specify the ARN of the certificates for all Ingresses that belong to IngressClass with this IngressClassParams.

If the field is specified, LBC will ignore the `alb.ingress.kubernetes.io/certificate-arn` annotation.

#### spec.sslPolicy

Cluster administrators can use the optional `sslPolicy` field to specify the SSL policy for the load balancers that belong to this IngressClass.
If the field is specified, LBC will ignore the `alb.ingress.kubernetes.io/ssl-policy` annotation.

#### spec.subnets

Cluster administrators can use the optional `subnets` field to specify the subnets for the load balancers that belong to this IngressClass.
They may specify either `ids` or `tags`. If the field is specified, LBC will ignore the `alb.ingress.kubernetes.io/subnets annotation` annotation.

##### spec.subnets.ids

If `ids` is specified, it must be a set of at least one resource ID of a subnet in the VPC. No two subnets may be in the same availability zone.

##### spec.subnets.tags

If `tags` is specified, it is a map of tag filters. The filters will match subnets in the VPC for which
each listed tag key is present and has one of the corresponding tag values.

Unless the `SubnetsClusterTagCheck` feature gate is disabled, subnets without a cluster tag and with the cluster tag for another cluster will be excluded.

Within any given availability zone, subnets with a cluster tag will be chosen over subnets without, then the subnet with the lowest-sorting resource ID will be chosen.

#### spec.ipAddressType

`ipAddressType` is an optional setting. The available options are `ipv4`, `dualstack`, or `dualstack-without-public-ipv4`.

Cluster administrators can use `ipAddressType` field to restrict the ipAddressType for all Ingresses that belong to this IngressClass.

1. If `ipAddressType` specified, all Ingresses with this IngressClass will have the specified ipAddressType.
2. If `ipAddressType` un-specified, Ingresses with this IngressClass can continue to use `alb.ingress.kubernetes.io/ip-address-type` annotation to specify ipAddressType.

#### spec.tags

`tags` is an optional setting.

Cluster administrators can use `tags` field to specify the custom tags for AWS resources provisioned for all Ingresses belong to this IngressClass.

1. If `tags` is set, AWS resources provisioned for all Ingresses with this IngressClass will have the specified tags.
2. You can also use controller-level flag `--default-tags`  or `alb.ingress.kubernetes.io/tags` annotation to specify custom tags. These tags will be merged together based on tag-key. If same tag-key appears in multiple sources, the priority is as follows:
    1. controller-level flag `--default-tags` will have the highest priority.
    2. `spec.tags` in IngressClassParams will have the middle priority.
    3. `alb.ingress.kubernetes.io/tags` annotation will have the lowest priority.

#### spec.loadBalancerAttributes

`loadBalancerAttributes` is an optional setting.

Cluster administrators can use `loadBalancerAttributes` field to specify the [Load Balancer Attributes](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html#load-balancer-attributes) that should be applied to the load balancers that belong to this IngressClass. You can specify the list of load balancer attribute name and the desired value in the `spec.loadBalancerAttributes` field.

1. If `loadBalancerAttributes` is set, the attributes defined will be applied to the load balancer that belong to this IngressClass. If you specify invalid keys or values for the load balancer attributes, the controller will fail to reconcile ingresses belonging to the particular ingress class.
2. If `loadBalancerAttributes` un-specified, Ingresses with this IngressClass can continue to use `alb.ingress.kubernetes.io/load-balancer-attributes` annotation to specify the load balancer attributes.

#### spec.minimumLoadBalancerCapacity

Cluster administrators can use the optional `minimumLoadBalancerCapacity` field to specify the capacity reservation for the load balancers that belong to this IngressClass.
They may specify `capacityUnits`. If the field is specified, LBC will ignore the `alb.ingress.kubernetes.io/minimum-load-balancer-capacity annotation` annotation.

##### spec.minimumLoadBalancerCapacity.capacityUnits

If `capacityUnits` is specified, it must be to valid positive value greater than 0. If set to 0, the LBC will reset the capacity reservation for the load balancer.

#### spec.ipv4IPAMPoolId

The IPAM pool you choose will be the preferred source of public IPv4 addresses.
If the pool is depleted, IPv4 addresses will be assigned by AWS.
To remove the IPAM pool from your ALB, remove `spec.ipv4IPAMPoolId` from the IngressClass definition.

#### spec.prefixListIDs

`prefixListIDs` is an optional setting.

Cluster administrators can use `prefixListIDs` field to specify the managed prefix lists that are allowed to access the load balancers that belong to this IngressClass. You can specify the list of prefix list IDs in the `spec.prefixListIDs` field.

1. If `prefixListIDs` is set, the prefix lists defined will be applied to the load balancer that belong to this IngressClass. If you specify invalid prefix list IDs, the controller will fail to reconcile ingresses belonging to the particular ingress class.
2. If `prefixListIDs` un-specified, Ingresses with this IngressClass can continue to use `alb.ingress.kubernetes.io/security-group-prefix-lists` annotation to specify the load balancer prefix lists.

#### spec.authenticationConfiguration

Cluster administrators can use the optional `authenticationConfiguration` field to specify the authentication configuration for all load balancers that belong to this IngressClass. Application Load Balancer (ALB) supports authentication with Cognito or OIDC for HTTPS listeners.

For all authentication types, the following specifications are available:

- `type`
  - The authentication type on targets.
  - Value: `none`, `oidc`, `cognito`
  - Type: `string`
  - Required
- `idpCognitoConfiguration`
  - The Cognito IdP configuration.
  - Type: `object`
- `idpOidcConfiguration`
  - The OIDC IdP configuration.
  - Type: `object`
- `onUnauthenticatedRequest`
  - The behavior if the user is not authenticated.
  - Value: `authenticate`, `deny`, `allow`
  - Type: `string`
- `scope`
  - The set of user claims to be requested from the Cognito IDP or OIDC IDP, in a space-separated list.
    - Options: `phone`, `email`, `profile`, `openid`, `aws.cognito.signin.user.admin`
    - Ex. `'email openid'`
  - Type: `string`
- `sessionCookie`
  - The name of the cookie used to maintain session information.
  - Type: `string`
- `sessionTimeout`
  - The maximum duration of the authentication session, in seconds.
  - Type: `integer`

If the `authenticationConfiguration` type is `oidc`, then set the `idpOidcConfiguration` field with the following properties

- `authorizationEndpoint`
  - The authorization endpoint of the IdP.
  - Type: `string`
  - Required
- `issuer`
  - The OIDC issuer identifier of the IdP.
  - Type: `string`
  - Required
- `secretName`
  - The k8s secretName.
  - The secret must be created in the `default` namespace. It holds the OIDC `clientID` and `clientSecret`.
  - Example format:
    ```
    apiVersion: v1
    kind: Secret
    metadata:
      namespace: default
      name: my-k8s-secret
    data:
      clientID: base64 of your plain text clientId
      clientSecret: base64 of your plain text clientSecret
    ```
  - Type: `string`
  - Required
- `tokenEndpoint`
  - The token endpoint of the IdP.
  - Type: `string`
  - Required
- `userInfoEndpoint`
  - The user info endpoint of the IdP.
  - Type: `string`
  - Required
- `additionalProperties`
  - The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
  - Type: `object`

If the `authenticationConfiguration` type is `cognito`, then set the `idpCognitoConfiguration` field with the following properties

- `authenticationRequestExtraParams`
  - The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
  - Type: `object`
  - Required
- `userPoolARN`
  - The Amazon Resource Name (ARN) of the Amazon Cognito user pool.
  - Type: `string`
  - Required
- `userPoolClientID`
  - The ID of the Amazon Cognito user pool client.
  - Type: `string`
  - Required
- `userPoolDomain`
  - The domain prefix or fully-qualified domain name of the Amazon Cognito user pool.
    - If you are using Amazon Cognito Domain, the userPoolDomain should be set to the domain prefix (ex. my-domain) instead of full domain (ex. https://my-domain.auth.us-west-2.amazoncognito.com).
  - Type: `string`
  - Required
- `additionalProperties`
  - The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
  - Type: `object`

To remove the IngressClass authentication configuration from your ALB, remove `spec.authenticationConfiguration` from the IngressClass definition.

When `spec.authenticationConfiguration` is specified, LBC will ignore the following Ingress annotations:

- `alb.ingress.kubernetes.io/auth-type`
- `alb.ingress.kubernetes.io/auth-idp-cognito`
- `alb.ingress.kubernetes.io/auth-idp-oidc`
- `alb.ingress.kubernetes.io/auth-on-unauthenticated-request`
- `alb.ingress.kubernetes.io/auth-scope`
- `alb.ingress.kubernetes.io/auth-session-cookie`
- `alb.ingress.kubernetes.io/auth-session-timeout`
