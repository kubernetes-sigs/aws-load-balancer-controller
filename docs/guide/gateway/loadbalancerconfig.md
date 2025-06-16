## LoadBalancerConfiguration

### Top Level Fields

#### MergingMode

`mergingMode`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  mergingMode: prefer-gateway-class
```

Defines the merge behavior when both the Gateway and GatewayClass have a defined LoadBalancerConfiguration. This field is only honored for the configuration attached to the GatewayClass.

* **Options**:
    - prefer-gateway-class: When merging configuration from both Gateway and GatewayClass, value conflicts are resolved by using the GatewayClass configuration.
    - prefer-gateway: When merging configuration from both Gateway and GatewayClass, value conflicts are resolved by using the Gateway configuration.

**Default** prefer-gateway-class

#### LoadBalancerName

`loadBalancerName`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  loadBalancerName: foo
```

Defines the name of the LB to provision. If unspecified, it will be automatically generated.

**Default** Autogenerate Name

#### Scheme

`scheme`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  scheme: internal
```

Defines the LoadBalancer Scheme.

- internal
  - the LB is only accessible within the VPC.
- internet-facing
  - the LB is accessible via the public internet.

**Default** internal

#### IpAddressType

`ipAddressType`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  ipAddressType: dualstack
```

Define what IP Address Type to use.

- ipv4
  - Only publish IPv4 address(es)
- dualstack
  - Publish both IPv4 and IPv6 address(es)
- dualstack-without-public-ipv4
  - Publish private IPv4 address(es) and public IPv6 address(es)
  - Only applicable to ALB Gateways

**Default** ipv4


#### EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic

`enforceSecurityGroupInboundRulesOnPrivateLinkTraffic`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  enforceSecurityGroupInboundRulesOnPrivateLinkTraffic: on
```

Indicates whether to evaluate inbound security group rules for traffic sent to a Network Load Balancer through Amazon Web Services PrivateLink.

Valid options are `on` and `off`

Only applicable to NLB Gateways.

**Default** on

#### CustomerOwnedIpv4Pool

`customerOwnedIpv4Pool`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  customerOwnedIpv4Pool: coip-1234
```

The ID of the customer-owned address for Application Load Balancers on Outposts pool.

Only applicable to ALB Gateways.

**Default** no value

#### IPv4IPAMPoolId

`ipv4IPAMPoolId`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  ipv4IPAMPoolId: ipam-1234
```

The IPAM pool ID used for IPv4 Addresses on the ALB.

Only applicable to ALB Gateways.

**Default** no value

#### LoadBalancerSubnets

`loadBalancerSubnets`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  loadBalancerSubnets:
    - identifier: subnet-1234
```

An optional list of subnet configurations to be used in the LB. This value takes precedence over subnet `loadBalancerSubnetsSelector` if both are selected.

See [SubnetConfiguration](#SubnetConfiguration) for more more details


**Default** Use [Subnet Discovery](../../deploy/subnet_discovery.md)


#### LoadBalancerSubnetsSelector

`loadBalancerSubnetsSelector`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  loadBalancerSubnetsSelector:
    key1:
      - k1
      - k2
      - k3
```

The subnets in the load balancer's VPC where each tag specified in the map key contains one of the values in the corresponding value list.

**Default** Use [Subnet Discovery](../../deploy/subnet_discovery.md)

#### ListenerConfigurations

`listenerConfigurations`

A list of Listener Configurations. See the [ListenerConfiguration](#ListenerConfiguration)

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  listenerConfigurations:
    - protocolPort: TCP:80
      defaultCertificate: my-cert
    - protocolPort: TCP:81
      defaultCertificate: my-cert1
```

**Default** Empty list

#### SecurityGroups

`securityGroups`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  securityGroups:
    - "sg1"
    - "sg2"
```

If you specify this annotation, you need to configure the security groups on your Node/Pod to allow inbound traffic from the load balancer. 
You could also set the [`manage-backend-security-group-rules`](#ManageBackendSecurityGroupRules) if you want the controller to manage the access rules.
Both name or ID of securityGroups are supported. Name matches a `Name` tag, not the `groupName` attribute.

**Default**

The controller will automatically create one security group, the security group will be attached to the LoadBalancer and allow access from [`SourceRanges`](#SourceRanges) and [`SecurityGroupPrefixes`](#SecurityGroupPrefixes) to each Listener port.
Also, the securityGroups for Node/Pod will be modified to allow inbound traffic from this securityGroup.

#### SecurityGroupPrefixes

`securityGroupPrefixes`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  securityGroupPrefixes:
    - "pl1"
    - "pl2"
```

An optional list of prefixes that are allowed to access the LB.

**Default** Empty list

#### SourceRanges

`sourceRanges`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  sourceRanges:
    - "2001:db8::/32"
    - "127.0.0.1/24"
```

An optional list of CIDRs that are allowed to access the LB.

**Default** Empty list


#### VpcId

`vpcId`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  vpcId: vpc-1234
```

The VPC ID of LoadBalancer

**Default** Autodetect VPC from Cluster VPC

#### LoadBalancerAttributes

`loadBalancerAttributes`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  loadBalancerAttributes:
    - key: k1
      value: v1
    - key: k2
      value: v2
```

The attributes to apply to an LB.
See the [ELB documentation](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_LoadBalancerAttribute.html) for a full list of attributes

**Default** Empty list

#### Tags

`tags`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  tags:
    tag-k1: v1
    tag-k2: v2
```

The tags to apply to an LB.

**Default** No tags

#### EnableICMP

`enableICMP`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  enableICMP: true
```

Enables the creation of security group rules to the managed security group to allow explicit ICMP traffic for Path MTU discovery for IPv4 and dual-stack VPCs

Only applies to Network LoadBalancers.

**Default** false

#### ManageBackendSecurityGroupRules

`manageBackendSecurityGroupRules`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  manageBackendSecurityGroupRules: true
```

Specify whether you want the controller to configure security group rules on the Node/Pod for traffic access when you specify securityGroups

**Default** false

#### MinimumLoadBalancerCapacity

`minimumLoadBalancerCapacity`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  minimumLoadBalancerCapacity:
    capacityUnits: 100000
```

Define the [capacity reservation](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/capacity-unit-reservation.html) for LoadBalancers

**Default** No capacity reservation

### ListenerConfiguration

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  listenerConfigurations:
    - protocolPort: TCP:80
      defaultCertificate: my-cert
      certificates: [arn-1, arn2, arn3]
      sslPolicy: ELBSecurityPolicy-2016-08
      alpnPolicy: HTTP1Only
```

#### ProtocolPort

`protocolPort`

The identifier for the listener on load balancer. It should be of the form PROTOCOL:PORT

**Default** No default, not an optional field.

#### DefaultCertificate

`defaultCertificate`

The default cert ARN.

**Default** No cert

#### Certificates

`certificates`

A list of cert ARNs to accept on this listener.

**Default** Empty list

#### SslPolicy

`sslPolicy`

The security policy that defines which protocols and ciphers are supported for secure listeners [HTTPS or TLS listener].

See the documentation for more information
[ALB](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/describe-ssl-policies.html)
[NLB](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/describe-ssl-policies.html)

**Default** ELBSecurityPolicy-2016-08

#### ALPNPolicy

`alpnPolicy`

An optional string that allows you to configure ALPN policies on your Load Balancer.

See the documentation for more details:
[ALPN](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-listeners.html#alpn-policies)

Possible values:
- HTTP1Only 
  - Negotiate only HTTP/1.*. The ALPN preference list is http/1.1, http/1.0.
- HTTP2Only
  - Negotiate only HTTP/2. The ALPN preference list is h2.
- HTTP2Optional
  - Prefer HTTP/1.* over HTTP/2 (which can be useful for HTTP/2 testing). The ALPN preference list is http/1.1, http/1.0, h2.
- HTTP2Preferred
  - Prefer HTTP/2 over HTTP/1.*. The ALPN preference list is h2, http/1.1, http/1.0.
- None
  - Do not negotiate ALPN.

Only applies to Network LoadBalancer Gateways.

**Default** None

#### MutualAuthentication

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  listenerConfigurations:
    - protocolPort: TCP:80
      defaultCertificate: my-cert
      certificates: [arn-1, arn2, arn3]
      mutualAuthentication:
        advertiseTrustStoreCaNames: "off"
        ignoreClientCertificateExpiry: true
        mode: verify
        trustStore: ts-1234
```

`mutualAuthentication`

Define the mutual authentication configuration information. Using [MutualAuthenticationAttributes](#MutualAuthenticationAttributes) 

See the documentation for more information
[mTLS](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/mutual-authentication.html)

Only applies to Application LoadBalancer Gateways.

**Default** No MTLS

#### ListenerAttributes

`listenerAttributes`

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  listenerConfigurations:
    - protocolPort: TCP:80
      defaultCertificate: my-cert
      certificates: [arn-1, arn2, arn3]
      listenerAttributes:
        - key: k1
          value: v1
        - key: k2
          value: v2
```

Define the attributes for the listener.

See [ListenerAttributes](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_ListenerAttribute.html)
for a complete list.

**Default** Empty list

### MutualAuthenticationAttributes

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  listenerConfigurations:
    - protocolPort: TCP:80
      defaultCertificate: my-cert
      certificates: [arn-1, arn2, arn3]
      mutualAuthentication:
        advertiseTrustStoreCaNames: "off"
        ignoreClientCertificateExpiry: true
        mode: verify
        trustStore: ts-1234
```

#### AdvertiseTrustStoreCaNames

`advertiseTrustStoreCaNames`

Whether trust store CA certificate names are advertised. Supported values are "on" and "off"

**Default** "off"

#### IgnoreClientCertificateExpiry

`ignoreClientCertificateExpiry`

Whether expired client certificates are ignored.

**Default** False

#### MutualAuthenticationMode

`mode`

The client certificate handling method

Possible values:
- verify
  - When you use mutual TLS verify mode, Application Load Balancer performs X.509 client certificate authentication for clients when a load balancer negotiates TLS connections
- passthrough
  - When you use mutual TLS passthrough mode, Application Load Balancer sends the whole client certificate chain to the target using HTTP headers. Then, by using the client certificate chain, you can implement corresponding load balancer authentication and target authorization logic in your application.
- off
  - mTLS is not enabled.

**Default** Off

#### TrustStore

`trustStore`

The Name or ARN of the trust store.

**Default** Empty string

### SubnetConfiguration

```
apiVersion: gateway.k8s.aws/v1beta1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  loadBalancerSubnets:
    - identifier: "my-subnet"
      eipAllocation: "eip-1234"
      privateIPv4Allocation: "127.0.0.1"
      ipv6Allocation: "69e1:9570:c975:1af1:8073:384c:5aae:53c6"
      sourceNatIPv6Prefix: "2001:db8::/32"
```

#### Identifier

`identifier`

The name or ID of the subnet

**Default** Empty string

#### EIPAllocation

`eipAllocation`

The EIP name for this subnet.

Only applies to Network LoadBalancer Gateways.

**Default** Empty string

#### PrivateIPv4Allocation

`privateIPv4Allocation`

The private ipv4 address to assign to this subnet.

Only applies to Network LoadBalancer Gateways.

**Default** Empty string

#### IPv6Allocation

`ipv6Allocation`

The ipv6 address to assign to this subnet.

Only applies to Network LoadBalancer Gateways.

**Default** Empty string

#### SourceNatIPv6Prefix

`sourceNatIPv6Prefix`

The IPv6 prefix to use for source NAT. Specify an IPv6 prefix (/80 netmask) from the subnet CIDR block or auto_assigned to use an IPv6 prefix selected at random from the subnet CIDR block.

Only applies to Network LoadBalancer Gateways.

**Default** Empty string