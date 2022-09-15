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

### IngressClassParams specification

#### spec.namespaceSelector
`namespaceSelector` is an optional setting that follows general Kubernetes
[label selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors)
semantics.

Cluster administrators can use the `namespaceSelector` field to restrict the namespaces of Ingresses that are allowed to specify the IngressClass.

1. If `namespaceSelector` specified, only Ingresses in selected namespaces can use IngressClasses with this parameter. The controller will refuse to reconcile for Ingresses that violates `namespaceSelector`.
2. If `namespaceSelector` un-specified, all Ingresses in any namespace can use IngressClasses with this parameter.

#### spec.group

`group` is an optional setting.  The only available sub-field is `group.name`.

Cluster administrators can use `group.name` field to denote the groupName for all Ingresses belong to this IngressClass.

1. If `group.name` specified, all Ingresses with this IngressClass will belong to the same IngressGroup specified and result in a single ALB.
If `group.name` is not specified, Ingresses with this IngressClass can use the older / legacy `alb.ingress.kubernetes.io/group.name` annotation to specify their IngressGroup. Ingresses that belong to the same IngressClass can form different IngressGroups via that annotation.

#### spec.scheme

`scheme` is an optional setting. The available options are `internet-facing` or `internal`.

Cluster administrators can use the `scheme` field to restrict the scheme for all Ingresses that belong to this IngressClass.

1. If `scheme` specified, all Ingresses with this IngressClass will have the specified scheme.
2. If `scheme` un-specified, Ingresses with this IngressClass can continue to use `alb.ingress.kubernetes.io/scheme annotation` to specify scheme.

#### spec.ipAddressType

`ipAddressType` is an optional setting. The available options are `ipv4` or `dualstack`.

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
