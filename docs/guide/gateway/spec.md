# API Reference

## Packages
- [gateway.k8s.aws/v1beta1](#gatewayk8sawsv1beta1)


## gateway.k8s.aws/v1beta1

Package v1beta1 contains API Schema definitions for the elbv2 v1beta1 API group

### Resource Types
- [ListenerRuleConfiguration](#listenerruleconfiguration)
- [LoadBalancerConfiguration](#loadbalancerconfiguration)
- [TargetGroupConfiguration](#targetgroupconfiguration)



#### ALPNPolicy

_Underlying type:_ _string_

ALPNPolicy defines the ALPN policy configuration for TLS listeners forwarding to TLS target groups
HTTP1Only Negotiate only HTTP/1.*. The ALPN preference list is http/1.1, http/1.0.
HTTP2Only Negotiate only HTTP/2. The ALPN preference list is h2.
HTTP2Optional Prefer HTTP/1.* over HTTP/2 (which can be useful for HTTP/2 testing). The ALPN preference list is http/1.1, http/1.0, h2.
HTTP2Preferred Prefer HTTP/2 over HTTP/1.*. The ALPN preference list is h2, http/1.1, http/1.0.
None Do not negotiate ALPN. This is the default.

_Validation:_
- Enum: [HTTP1Only HTTP2Only HTTP2Optional HTTP2Preferred None]

_Appears in:_
- [ListenerConfiguration](#listenerconfiguration)

| Field | Description |
| --- | --- |
| `None` |  |
| `HTTP1Only` |  |
| `HTTP2Only` |  |
| `HTTP2Optional` |  |
| `HTTP2Preferred` |  |


#### Action



Action defines an action for a listener rule



_Appears in:_
- [ListenerRuleSpec](#listenerrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[ActionType](#actiontype)_ | The type of action |  | Enum: [forward fixed-response redirect authenticate-cognito authenticate-oidc] <br /> |
| `forwardConfig` _[ForwardActionConfig](#forwardactionconfig)_ | Information for a forward action |  |  |
| `redirectConfig` _[RedirectActionConfig](#redirectactionconfig)_ | Information for a redirect action |  |  |
| `fixedResponseConfig` _[FixedResponseActionConfig](#fixedresponseactionconfig)_ | Information for a fixed-response action |  |  |
| `authenticateCognitoConfig` _[AuthenticateCognitoActionConfig](#authenticatecognitoactionconfig)_ | Information for an authenticate-cognito action |  |  |
| `authenticateOIDCConfig` _[AuthenticateOidcActionConfig](#authenticateoidcactionconfig)_ | Information for an authenticate-oidc action |  |  |


#### ActionType

_Underlying type:_ _string_

ActionType defines the type of action for the rule

_Validation:_
- Enum: [forward fixed-response redirect authenticate-cognito authenticate-oidc]

_Appears in:_
- [Action](#action)

| Field | Description |
| --- | --- |
| `forward` |  |
| `fixed-response` |  |
| `redirect` |  |
| `authenticate-cognito` |  |
| `authenticate-oidc` |  |


#### AdvertiseTrustStoreCaNamesEnum

_Underlying type:_ _string_



_Validation:_
- Enum: [on off]

_Appears in:_
- [MutualAuthenticationAttributes](#mutualauthenticationattributes)

| Field | Description |
| --- | --- |
| `on` |  |
| `off` |  |


#### AuthenticateCognitoActionConditionalBehaviorEnum

_Underlying type:_ _string_

AuthenticateCognitoActionConditionalBehaviorEnum defines the behavior when a user is not authenticated

_Validation:_
- Enum: [deny allow authenticate]

_Appears in:_
- [AuthenticateCognitoActionConfig](#authenticatecognitoactionconfig)

| Field | Description |
| --- | --- |
| `deny` |  |
| `allow` |  |
| `authenticate` |  |


#### AuthenticateCognitoActionConfig



Information about an authenticate-cognito action



_Appears in:_
- [Action](#action)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userPoolArn` _string_ | The Amazon Resource Name (ARN) of the Amazon Cognito user pool. |  |  |
| `userPoolClientId` _string_ | The ID of the Amazon Cognito user pool client. |  |  |
| `userPoolDomain` _string_ | The domain prefix or fully-qualified domain name of the Amazon Cognito user<br />pool. |  |  |
| `scope` _string_ | The set of user claims to be requested from the IdP. The default is openid .<br /><br />To verify which scope values your IdP supports and how to separate multiple<br />values, see the documentation for your IdP. | openid |  |
| `authenticationRequestExtraParams` _map[string]string_ | The query parameters (up to 10) to include in the redirect request to the<br />authorization endpoint. |  | MaxProperties: 10 <br /> |
| `onUnauthenticatedRequest` _[AuthenticateCognitoActionConditionalBehaviorEnum](#authenticatecognitoactionconditionalbehaviorenum)_ | The behavior if the user is not authenticated. The following are possible | authenticate | Enum: [deny allow authenticate] <br /> |
| `sessionCookieName` _string_ | The name of the cookie used to maintain session information. The default is<br />AWSELBAuthSessionCookie. | AWSELBAuthSessionCookie |  |
| `sessionTimeout` _integer_ | The maximum duration of the authentication session, in seconds. The default is<br />604800 seconds (7 days). | 604800 | Maximum: 604800 <br />Minimum: 1 <br /> |


#### AuthenticateOidcActionConditionalBehaviorEnum

_Underlying type:_ _string_

AuthenticateOidcActionConditionalBehaviorEnum defines the behavior when a user is not authenticated

_Validation:_
- Enum: [deny allow authenticate]

_Appears in:_
- [AuthenticateOidcActionConfig](#authenticateoidcactionconfig)

| Field | Description |
| --- | --- |
| `deny` |  |
| `allow` |  |
| `authenticate` |  |


#### AuthenticateOidcActionConfig



Information about an authenticate-oidc action



_Appears in:_
- [Action](#action)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `authorizationEndpoint` _string_ | The authorization endpoint of the IdP. This must be a full URL, including the<br />HTTPS protocol, the domain, and the path. |  |  |
| `secret` _[Secret](#secret)_ | Secret holds OAuth 2.0 clientID and clientSecret. You need to create this secret and provide its name and namespace |  |  |
| `issuer` _string_ | The OIDC issuer identifier of the IdP. This must be a full URL, including the<br />HTTPS protocol, the domain, and the path. |  |  |
| `tokenEndpoint` _string_ | The token endpoint of the IdP. This must be a full URL, including the HTTPS<br />protocol, the domain, and the path. |  |  |
| `userInfoEndpoint` _string_ | The user info endpoint of the IdP. This must be a full URL, including the HTTPS<br />protocol, the domain, and the path. |  |  |
| `scope` _string_ | The set of user claims to be requested from the IdP. The default is openid .<br /><br />To verify which scope values your IdP supports and how to separate multiple<br />values, see the documentation for your IdP. | openid |  |
| `authenticationRequestExtraParams` _map[string]string_ | The query parameters (up to 10) to include in the redirect request to the<br />authorization endpoint. |  | MaxProperties: 10 <br /> |
| `onUnauthenticatedRequest` _[AuthenticateOidcActionConditionalBehaviorEnum](#authenticateoidcactionconditionalbehaviorenum)_ | The behavior if the user is not authenticated. The following are possible | authenticate | Enum: [deny allow authenticate] <br /> |
| `sessionCookieName` _string_ | The name of the cookie used to maintain session information. The default is<br />AWSELBAuthSessionCookie. | AWSELBAuthSessionCookie |  |
| `sessionTimeout` _integer_ | The maximum duration of the authentication session, in seconds. The default is<br />604800 seconds (7 days). | 604800 | Maximum: 604800 <br />Minimum: 1 <br /> |
| `useExistingClientSecret` _boolean_ | Indicates whether to use the existing client secret when modifying a listener rule. If<br />you are creating a listener rule, you can omit this parameter or set it to false. |  |  |


#### FixedResponseActionConfig



Information about an action that returns a custom HTTP response.



_Appears in:_
- [Action](#action)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `statusCode` _integer_ | The HTTP response code (2XX, 4XX, or 5XX). |  |  |
| `contentType` _string_ | The content type of the fixed response. | text/plain | Enum: [text/plain text/css text/html application/javascript application/json] <br /> |
| `messageBody` _string_ | The message |  |  |


#### ForwardActionConfig



Information about a forward action.



_Appears in:_
- [Action](#action)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targetGroupStickinessConfig` _[TargetGroupStickinessConfig](#targetgroupstickinessconfig)_ | The target group stickiness for the rule.<br />Note: ForwardActionConfig only supports target group stickiness configuration through CRD.<br />All other forward action fields must be set through the Gateway API native way. | \{  \} |  |


#### HealthCheckConfiguration



HealthCheckConfiguration defines the Health Check configuration for a Target Group.



_Appears in:_
- [TargetGroupProps](#targetgroupprops)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `healthyThresholdCount` _integer_ | healthyThresholdCount The number of consecutive health checks successes required before considering an unhealthy target healthy. |  |  |
| `healthCheckInterval` _integer_ | healthCheckInterval The approximate amount of time, in seconds, between health checks of an individual target. |  |  |
| `healthCheckPath` _string_ | healthCheckPath The destination for health checks on the targets. |  |  |
| `healthCheckPort` _string_ | healthCheckPort The port the load balancer uses when performing health checks on targets.<br />The default is to use the port on which each target receives traffic from the load balancer. |  |  |
| `healthCheckProtocol` _[TargetGroupHealthCheckProtocol](#targetgrouphealthcheckprotocol)_ | healthCheckProtocol The protocol to use to connect with the target. The GENEVE, TLS, UDP, and TCP_UDP protocols are not supported for health checks. |  | Enum: [http https tcp] <br /> |
| `healthCheckTimeout` _integer_ | healthCheckTimeout The amount of time, in seconds, during which no response means a failed health check |  |  |
| `unhealthyThresholdCount` _integer_ | unhealthyThresholdCount The number of consecutive health check failures required before considering the target unhealthy. |  |  |
| `matcher` _[HealthCheckMatcher](#healthcheckmatcher)_ | healthCheckCodes The HTTP or gRPC codes to use when checking for a successful response from a target |  |  |


#### HealthCheckMatcher



TODO: Add a validation in the admission webhook to check if only one of HTTPCode or GRPCCode is set.
Information to use when checking for a successful response from a target.



_Appears in:_
- [HealthCheckConfiguration](#healthcheckconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `httpCode` _string_ | The HTTP codes. |  |  |
| `grpcCode` _string_ | The gRPC codes |  |  |


#### ListenerAttribute



ListenerAttribute defines listener attribute.



_Appears in:_
- [ListenerConfiguration](#listenerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ | The key of the attribute. |  |  |
| `value` _string_ | The value of the attribute. |  |  |


#### ListenerConfiguration







_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `protocolPort` _[ProtocolPort](#protocolport)_ | protocolPort is identifier for the listener on load balancer. It should be of the form PROTOCOL:PORT |  | Pattern: `^(HTTP\|HTTPS\|TLS\|TCP\|UDP)?:(6553[0-5]\|655[0-2]\d\|65[0-4]\d\{2\}\|6[0-4]\d\{3\}\|[1-5]\d\{4\}\|[1-9]\d\{0,3\})?$` <br /> |
| `defaultCertificate` _string_ | defaultCertificate the cert arn to be used by default. |  |  |
| `certificates` _string array_ | certificates is the list of other certificates to add to the listener. |  |  |
| `sslPolicy` _string_ | sslPolicy is the security policy that defines which protocols and ciphers are supported for secure listeners [HTTPS or TLS listener]. |  |  |
| `alpnPolicy` _[ALPNPolicy](#alpnpolicy)_ | alpnPolicy an optional string that allows you to configure ALPN policies on your Load Balancer | None | Enum: [HTTP1Only HTTP2Only HTTP2Optional HTTP2Preferred None] <br /> |
| `mutualAuthentication` _[MutualAuthenticationAttributes](#mutualauthenticationattributes)_ | mutualAuthentication defines the mutual authentication configuration information. | \{ mode:off \} |  |
| `listenerAttributes` _[ListenerAttribute](#listenerattribute) array_ | listenerAttributes defines the attributes for the listener |  |  |


#### ListenerRuleCondition



Information about a condition for a listener rule



_Appears in:_
- [ListenerRuleSpec](#listenerrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `field` _[ListenerRuleConditionField](#listenerruleconditionfield)_ | The field in the HTTP request |  | Enum: [source-ip] <br /> |
| `sourceIPConfig` _[SourceIPConditionConfig](#sourceipconditionconfig)_ | Information for a source IP condition |  |  |


#### ListenerRuleConditionField

_Underlying type:_ _string_

ListenerRuleConditionField defines the field in the HTTP request to match

_Validation:_
- Enum: [source-ip]

_Appears in:_
- [ListenerRuleCondition](#listenerrulecondition)

| Field | Description |
| --- | --- |
| `source-ip` |  |


#### ListenerRuleConfiguration



ListenerRuleConfiguration is the Schema for the ListenerRuleConfiguration API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `gateway.k8s.aws/v1beta1` | | |
| `kind` _string_ | `ListenerRuleConfiguration` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ListenerRuleSpec](#listenerrulespec)_ |  |  |  |
| `status` _[ListenerRuleStatus](#listenerrulestatus)_ |  |  |  |


#### ListenerRuleSpec



ListenerRuleSpec defines the desired state of ListenerRuleConfiguration



_Appears in:_
- [ListenerRuleConfiguration](#listenerruleconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `actions` _[Action](#action) array_ | Actions defines the set of actions to be performed when conditions match.<br />This CRD implementation currently supports only  authenticate-oidc, authenticate-cognito, and fixed-response action types fully and forward and redirect actions partially<br /><br />For other fields in forward and redirect actions, please use the standard Gateway API HTTPRoute or other route resources, which provide<br />native support for those conditions through the Gateway API specification.<br /><br />At most one authentication action can be specified (either authenticate-oidc or authenticate-cognito). |  | MaxItems: 2 <br />MinItems: 1 <br /> |
| `conditions` _[ListenerRuleCondition](#listenerrulecondition) array_ | Conditions defines the circumstances under which the rule actions will be performed.<br />This CRD implementation currently supports only the source-ip condition type<br /><br />For other condition types (such as path-pattern, host-header, http-header, etc.),<br />please use the standard Gateway API HTTPRoute or other route resources, which provide<br />native support for those conditions through the Gateway API specification. |  | MinItems: 1 <br /> |
| `tags` _map[string]string_ | Tags are the AWS resource tags to be applied to all AWS resources created for this rule. |  |  |


#### ListenerRuleStatus



ListenerRuleStatus defines the observed state of ListenerRuleConfiguration



_Appears in:_
- [ListenerRuleConfiguration](#listenerruleconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | The observed generation of the rule configuration |  |  |


#### LoadBalancerAttribute



LoadBalancerAttribute defines LB attribute.



_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ | The key of the attribute. |  |  |
| `value` _string_ | The value of the attribute. |  |  |


#### LoadBalancerConfigMergeMode

_Underlying type:_ _string_

LoadBalancerConfigMergeMode is the merging behavior defined when both Gateway and GatewayClass have lb configurations. See the individual
configuration fields for the exact merge behavior applied.

_Validation:_
- Enum: [prefer-gateway prefer-gateway-class]

_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description |
| --- | --- |
| `prefer-gateway-class` | MergeModePreferGatewayClass when both lb configurations have a field specified, this mode gives precedence to the configuration in the GatewayClass<br /> |
| `prefer-gateway` | MergeModePreferGatewayClass when both lb configurations have a field specified, this mode gives precedence to the configuration in the Gateway<br /> |


#### LoadBalancerConfiguration



LoadBalancerConfiguration is the Schema for the LoadBalancerConfiguration API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `gateway.k8s.aws/v1beta1` | | |
| `kind` _string_ | `LoadBalancerConfiguration` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)_ |  |  |  |
| `status` _[LoadBalancerConfigurationStatus](#loadbalancerconfigurationstatus)_ |  |  |  |


#### LoadBalancerConfigurationSpec



LoadBalancerConfigurationSpec defines the desired state of LoadBalancerConfiguration



_Appears in:_
- [LoadBalancerConfiguration](#loadbalancerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mergingMode` _[LoadBalancerConfigMergeMode](#loadbalancerconfigmergemode)_ | mergingMode defines the merge behavior when both the Gateway and GatewayClass have a defined LoadBalancerConfiguration.<br />This field is only honored for the configuration attached to the GatewayClass. |  | Enum: [prefer-gateway prefer-gateway-class] <br /> |
| `loadBalancerName` _string_ | loadBalancerName defines the name of the LB to provision. If unspecified, it will be automatically generated. |  | MaxLength: 32 <br />MinLength: 1 <br /> |
| `scheme` _[LoadBalancerScheme](#loadbalancerscheme)_ | scheme defines the type of LB to provision. If unspecified, it will be automatically inferred. |  | Enum: [internal internet-facing] <br /> |
| `ipAddressType` _[LoadBalancerIpAddressType](#loadbalanceripaddresstype)_ | loadBalancerIPType defines what kind of load balancer to provision (ipv4, dual stack) |  | Enum: [ipv4 dualstack dualstack-without-public-ipv4] <br /> |
| `enforceSecurityGroupInboundRulesOnPrivateLinkTraffic` _string_ | enforceSecurityGroupInboundRulesOnPrivateLinkTraffic Indicates whether to evaluate inbound security group rules for traffic sent to a Network Load Balancer through Amazon Web Services PrivateLink. |  |  |
| `customerOwnedIpv4Pool` _string_ | customerOwnedIpv4Pool [Application LoadBalancer]<br />is the ID of the customer-owned address for Application Load Balancers on Outposts pool. |  |  |
| `ipv4IPAMPoolId` _string_ | IPv4IPAMPoolId [Application LoadBalancer]<br />defines the IPAM pool ID used for IPv4 Addresses on the ALB. |  |  |
| `loadBalancerSubnets` _[SubnetConfiguration](#subnetconfiguration)_ | loadBalancerSubnets is an optional list of subnet configurations to be used in the LB<br />This value takes precedence over loadBalancerSubnetsSelector if both are selected. |  |  |
| `loadBalancerSubnetsSelector` _map[string][]string_ | LoadBalancerSubnetsSelector specifies subnets in the load balancer's VPC where each<br />tag specified in the map key contains one of the values in the corresponding<br />value list. |  |  |
| `listenerConfigurations` _[ListenerConfiguration](#listenerconfiguration)_ | listenerConfigurations is an optional list of configurations for each listener on LB |  |  |
| `securityGroups` _string_ | securityGroups an optional list of security group ids or names to apply to the LB |  |  |
| `securityGroupPrefixes` _string_ | securityGroupPrefixes an optional list of prefixes that are allowed to access the LB. |  |  |
| `sourceRanges` _string_ | sourceRanges an optional list of CIDRs that are allowed to access the LB. |  |  |
| `vpcId` _string_ | vpcId is the ID of the VPC for the load balancer. |  |  |
| `loadBalancerAttributes` _[LoadBalancerAttribute](#loadbalancerattribute) array_ | LoadBalancerAttributes defines the attribute of LB |  |  |
| `tags` _map[string]string_ | Tags the AWS Tags on all related resources to the gateway. |  |  |
| `enableICMP` _boolean_ | EnableICMP [Network LoadBalancer]<br />enables the creation of security group rules to the managed security group<br />to allow explicit ICMP traffic for Path MTU discovery for IPv4 and dual-stack VPCs |  |  |
| `manageBackendSecurityGroupRules` _boolean_ | ManageBackendSecurityGroupRules [Application / Network LoadBalancer]<br />specifies whether you want the controller to configure security group rules on Node/Pod for traffic access<br />when you specify securityGroups |  |  |
| `minimumLoadBalancerCapacity` _[MinimumLoadBalancerCapacity](#minimumloadbalancercapacity)_ | MinimumLoadBalancerCapacity define the capacity reservation for LoadBalancers |  |  |
| `wafV2` _[WAFv2Configuration](#wafv2configuration)_ | WAFv2 define the AWS WAFv2 settings for a Gateway [Application Load Balancer] |  |  |
| `shieldConfiguration` _[ShieldConfiguration](#shieldconfiguration)_ | ShieldAdvanced define the AWS Shield settings for a Gateway [Application Load Balancer] |  |  |


#### LoadBalancerConfigurationStatus



LoadBalancerConfigurationStatus defines the observed state of TargetGroupBinding



_Appears in:_
- [LoadBalancerConfiguration](#loadbalancerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGatewayConfigurationGeneration` _integer_ | The generation of the Gateway Configuration attached to the Gateway object. |  |  |
| `observedGatewayClassConfigurationGeneration` _integer_ | The generation of the Gateway Configuration attached to the GatewayClass object. |  |  |


#### LoadBalancerIpAddressType

_Underlying type:_ _string_

LoadBalancerIpAddressType is the IP Address type of your LB.

_Validation:_
- Enum: [ipv4 dualstack dualstack-without-public-ipv4]

_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description |
| --- | --- |
| `ipv4` |  |
| `dualstack` |  |
| `dualstack-without-public-ipv4` |  |


#### LoadBalancerScheme

_Underlying type:_ _string_

LoadBalancerScheme is the scheme of your LB


* with `internal` scheme, the LB is only accessible within the VPC.
* with `internet-facing` scheme, the LB is accesible via the public internet.

_Validation:_
- Enum: [internal internet-facing]

_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description |
| --- | --- |
| `internal` |  |
| `internet-facing` |  |


#### MinimumLoadBalancerCapacity



MinimumLoadBalancerCapacity Information about a load balancer capacity reservation.



_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `capacityUnits` _integer_ | The Capacity Units Value. |  |  |


#### MutualAuthenticationAttributes



Information about the mutual authentication attributes of a listener.



_Appears in:_
- [ListenerConfiguration](#listenerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `advertiseTrustStoreCaNames` _[AdvertiseTrustStoreCaNamesEnum](#advertisetruststorecanamesenum)_ | Indicates whether trust store CA certificate names are advertised. |  | Enum: [on off] <br /> |
| `ignoreClientCertificateExpiry` _boolean_ | Indicates whether expired client certificates are ignored. |  |  |
| `mode` _[MutualAuthenticationMode](#mutualauthenticationmode)_ | The client certificate handling method. Options are off, passthrough or verify |  | Enum: [off passthrough verify] <br /> |
| `trustStore` _string_ | The Name or ARN of the trust store. |  |  |


#### MutualAuthenticationMode

_Underlying type:_ _string_

MutualAuthenticationMode mTLS mode for mutual TLS authentication config for listener

_Validation:_
- Enum: [off passthrough verify]

_Appears in:_
- [MutualAuthenticationAttributes](#mutualauthenticationattributes)

| Field | Description |
| --- | --- |
| `off` |  |
| `passthrough` |  |
| `verify` |  |


#### Protocol

_Underlying type:_ _string_



_Validation:_
- Enum: [HTTP HTTPS TCP TLS UDP TCP_UDP]

_Appears in:_
- [TargetGroupProps](#targetgroupprops)

| Field | Description |
| --- | --- |
| `HTTP` |  |
| `HTTPS` |  |
| `TCP` |  |
| `TLS` |  |
| `UDP` |  |
| `TCP_UDP` |  |


#### ProtocolPort

_Underlying type:_ _string_



_Validation:_
- Pattern: `^(HTTP|HTTPS|TLS|TCP|UDP)?:(6553[0-5]|655[0-2]\d|65[0-4]\d{2}|6[0-4]\d{3}|[1-5]\d{4}|[1-9]\d{0,3})?$`

_Appears in:_
- [ListenerConfiguration](#listenerconfiguration)



#### ProtocolVersion

_Underlying type:_ _string_



_Validation:_
- Enum: [HTTP1 HTTP2 GRPC]

_Appears in:_
- [TargetGroupProps](#targetgroupprops)

| Field | Description |
| --- | --- |
| `HTTP1` |  |
| `HTTP2` |  |
| `GRPC` |  |


#### RedirectActionConfig



Information about a redirect action.



_Appears in:_
- [Action](#action)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `query` _string_ | The query parameters, URL-encoded when necessary, but not percent-encoded. Do<br />not include the leading "?", as it is automatically added. You can specify any<br />of the reserved keywords.<br />Note: RedirectActionConfig only supports setting the query parameter through CRD.<br />All other redirect action fields must be set through the Gateway API native way. | #\{query\} |  |


#### Reference



Reference defines how to look up the Target Group configuration for a service.



_Appears in:_
- [TargetGroupConfigurationSpec](#targetgroupconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `group` _string_ | Group is the group of the referent. For example, "gateway.networking.k8s.io".<br />When unspecified or empty string, core API group is inferred. |  |  |
| `kind` _string_ | Kind is the Kubernetes resource kind of the referent. For example<br />"Service".<br /><br />Defaults to "Service" when not specified. | Service |  |
| `name` _string_ | Name is the name of the referent. |  |  |


#### RouteConfiguration



RouteConfiguration defines the per route configuration



_Appears in:_
- [TargetGroupConfigurationSpec](#targetgroupconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `routeIdentifier` _[RouteIdentifier](#routeidentifier)_ | name the identifier of the route, it should be in the form of ROUTE:NAMESPACE:NAME |  |  |
| `targetGroupProps` _[TargetGroupProps](#targetgroupprops)_ | targetGroupProps the target group specific properties |  |  |


#### RouteIdentifier



RouteIdentifier the complete set of route attributes that identify a route.



_Appears in:_
- [RouteConfiguration](#routeconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ |  |  |  |
| `namespace` _string_ |  |  |  |
| `name` _string_ |  |  |  |


#### Secret



Secret holds OAuth 2.0 clientID and clientSecret. You need to create this secret and provide its name and namespace



_Appears in:_
- [AuthenticateOidcActionConfig](#authenticateoidcactionconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is name of the secret |  |  |
| `namespace` _string_ | Namespace is namespace of secret. If empty it will be considered to be in same namespace as of the resource referring it |  |  |


#### ShieldConfiguration



ShieldConfiguration configuration parameters used to configure Shield



_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled whether Shield Advanced should be configured with the Gateway |  |  |


#### SourceIPConditionConfig



Information about a source IP condition



_Appears in:_
- [ListenerRuleCondition](#listenerrulecondition)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `values` _string array_ | One or more source IP addresses, in CIDR format |  | MinItems: 1 <br /> |


#### SubnetConfiguration



SubnetConfiguration defines the subnet settings for a Load Balancer.



_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `identifier` _string_ | identifier [Application LoadBalancer / Network LoadBalancer] name or id for the subnet |  |  |
| `eipAllocation` _string_ | eipAllocation [Network LoadBalancer] the EIP name for this subnet. |  |  |
| `privateIPv4Allocation` _string_ | privateIPv4Allocation [Network LoadBalancer] the private ipv4 address to assign to this subnet. |  |  |
| `ipv6Allocation` _string_ | IPv6Allocation [Network LoadBalancer] the ipv6 address to assign to this subnet. |  |  |
| `sourceNatIPv6Prefix` _string_ | SourceNatIPv6Prefix [Network LoadBalancer] The IPv6 prefix to use for source NAT. Specify an IPv6 prefix (/80 netmask) from the subnet CIDR block or auto_assigned to use an IPv6 prefix selected at random from the subnet CIDR block. |  |  |


#### TargetGroupAttribute



TargetGroupAttribute defines target group attribute.



_Appears in:_
- [TargetGroupProps](#targetgroupprops)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ | The key of the attribute. |  |  |
| `value` _string_ | The value of the attribute. |  |  |




#### TargetGroupConfiguration



TargetGroupConfiguration is the Schema for defining TargetGroups with an AWS ELB Gateway





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `gateway.k8s.aws/v1beta1` | | |
| `kind` _string_ | `TargetGroupConfiguration` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[TargetGroupConfigurationSpec](#targetgroupconfigurationspec)_ |  |  |  |
| `status` _[TargetGroupConfigurationStatus](#targetgroupconfigurationstatus)_ |  |  |  |


#### TargetGroupConfigurationSpec



TargetGroupConfigurationSpec defines the TargetGroup properties for a route.



_Appears in:_
- [TargetGroupConfiguration](#targetgroupconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targetReference` _[Reference](#reference)_ | targetReference the kubernetes object to attach the Target Group settings to. |  |  |
| `defaultConfiguration` _[TargetGroupProps](#targetgroupprops)_ | defaultRouteConfiguration fallback configuration applied to all routes, unless overridden by route-specific configurations. |  |  |
| `routeConfigurations` _[RouteConfiguration](#routeconfiguration) array_ | routeConfigurations the route configuration for specific routes. the longest prefix match (kind:namespace:name) is taken to combine with the default properties. |  |  |


#### TargetGroupConfigurationStatus



TargetGroupConfigurationStatus defines the observed state of TargetGroupConfiguration



_Appears in:_
- [TargetGroupConfiguration](#targetgroupconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGatewayConfigurationGeneration` _integer_ | The generation of the Gateway Configuration attached to the Gateway object. |  |  |
| `observedGatewayClassConfigurationGeneration` _integer_ | The generation of the Gateway Configuration attached to the GatewayClass object. |  |  |


#### TargetGroupHealthCheckProtocol

_Underlying type:_ _string_



_Validation:_
- Enum: [http https tcp]

_Appears in:_
- [HealthCheckConfiguration](#healthcheckconfiguration)

| Field | Description |
| --- | --- |
| `HTTP` |  |
| `HTTPS` |  |
| `TCP` |  |


#### TargetGroupIPAddressType

_Underlying type:_ _string_

TargetGroupIPAddressType is the IP Address type of your ELBV2 TargetGroup.

_Validation:_
- Enum: [ipv4 ipv6]

_Appears in:_
- [TargetGroupProps](#targetgroupprops)

| Field | Description |
| --- | --- |
| `ipv4` |  |
| `ipv6` |  |


#### TargetGroupProps



TargetGroupProps defines the target group properties



_Appears in:_
- [RouteConfiguration](#routeconfiguration)
- [TargetGroupConfigurationSpec](#targetgroupconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targetGroupName` _string_ | targetGroupName specifies the name to assign to the Target Group. If not defined, then one is generated. |  |  |
| `ipAddressType` _[TargetGroupIPAddressType](#targetgroupipaddresstype)_ | ipAddressType specifies whether the target group is of type IPv4 or IPv6. If unspecified, it will be automatically inferred. |  | Enum: [ipv4 ipv6] <br /> |
| `healthCheckConfig` _[HealthCheckConfiguration](#healthcheckconfiguration)_ | healthCheckConfig The Health Check configuration for this backend. |  |  |
| `nodeSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#labelselector-v1-meta)_ | node selector for instance type target groups to only register certain nodes |  |  |
| `targetGroupAttributes` _[TargetGroupAttribute](#targetgroupattribute) array_ | targetGroupAttributes defines the attribute of target group |  |  |
| `targetType` _[TargetType](#targettype)_ | targetType is the TargetType of TargetGroup. If unspecified, it will be automatically inferred as instance. |  | Enum: [instance ip] <br /> |
| `protocol` _[Protocol](#protocol)_ | Protocol [Application / Network Load Balancer] the protocol for the target group.<br />If unspecified, it will be automatically inferred. |  | Enum: [HTTP HTTPS TCP TLS UDP TCP_UDP] <br /> |
| `protocolVersion` _[ProtocolVersion](#protocolversion)_ | protocolVersion [HTTP/HTTPS protocol] The protocol version. The possible values are GRPC , HTTP1 and HTTP2 |  | Enum: [HTTP1 HTTP2 GRPC] <br /> |
| `enableMultiCluster` _boolean_ | EnableMultiCluster [Application / Network LoadBalancer]<br />Allows for multiple Clusters / Services to use the generated TargetGroup ARN |  |  |
| `tags` _map[string]string_ | Tags the Tags to add on the target group. |  |  |


#### TargetGroupStickinessConfig



Information about the target group stickiness for a listener rule.



_Appears in:_
- [ForwardActionConfig](#forwardactionconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `durationSeconds` _integer_ | The time period, in seconds, during which requests from a client should be<br />routed to the same target group. The range is 1-604800 seconds (7 days). | 3600 | Maximum: 604800 <br />Minimum: 1 <br /> |
| `enabled` _boolean_ | Indicates whether target group stickiness is enabled. | false |  |


#### TargetType

_Underlying type:_ _string_

TargetType is the targetType of your ELBV2 TargetGroup.


* with `instance` TargetType, nodes with nodePort for your service will be registered as targets
* with `ip` TargetType, Pods with containerPort for your service will be registered as targets

_Validation:_
- Enum: [instance ip]

_Appears in:_
- [TargetGroupProps](#targetgroupprops)

| Field | Description |
| --- | --- |
| `instance` |  |
| `ip` |  |


#### WAFv2Configuration



WAFv2Configuration configuration parameters used to configure WAFv2



_Appears in:_
- [LoadBalancerConfigurationSpec](#loadbalancerconfigurationspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `webACL` _string_ | ACL The WebACL to configure with the Gateway |  |  |


