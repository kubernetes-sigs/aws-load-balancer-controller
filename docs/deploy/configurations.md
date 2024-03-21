# Controller configuration options
This document covers configuration of the AWS Load Balancer controller

!!!warning "limitation"
    The v2.0.0+ version of AWSLoadBalancerController currently only support one controller deployment(with one or multiple replicas) per cluster.

    The AWSLoadBalancerController assumes it's the solo owner of worker node security group rules with `elbv2.k8s.aws/targetGroupBinding=shared` description, running multiple controller deployment will cause these controllers compete with each other updating worker node security group rules.

    We will remove this limitation in future versions: [tracking issue](https://github.com/kubernetes-sigs/aws-load-balancer-controller/issues/2185)

## AWS API Access
To perform operations, the controller must have required IAM role capabilities for accessing and
provisioning ALB resources. There are many ways to achieve this, such as loading `AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY` as environment variables or using [kube2iam](https://github.com/jtblin/kube2iam).

Refer to the [installation guide](installation.md) for installing the controller in your kubernetes cluster and for the minimum required IAM permissions.

## Setting Ingress Resource Scope
You can limit the ingresses ALB ingress controller controls by combining following two approaches:

### Limiting ingress class
Setting the `--ingress-class` argument constrains the controller's scope to ingresses with matching `ingressClassName` field.

An example of the container spec portion of the controller, only listening for resources with the class "alb", would be as follows.

```yaml
spec:
  containers:
  - args:
    - --ingress-class=alb
```

Now, only ingress resources with the appropriate class are picked up, as seen below.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: echoserver
  namespace: echoserver
spec:
  ingressClassName: alb
  ...
```

If the ingress class is not specified, the controller will reconcile Ingress objects without the ingress class specified or ingress class `alb`.

### Limiting Namespaces
Setting the `--watch-namespace` argument constrains the controller's scope to a single namespace. Ingress events outside of the namespace specified are not be seen by the controller.

An example of the container spec, for a controller watching only the `default` namespace, is as follows.

```yaml
spec:
  containers:
  - args:
    - --watch-namespace=default
```

!!!note ""
Currently, you can set only 1 namespace to watch in this flag. See [this Kubernetes issue](https://github.com/kubernetes/contrib/issues/847) for more details.

## Controller command line flags

!!!warning ""
    The --cluster-name flag is mandatory and the value must match the name of the kubernetes cluster. If you specify an incorrect name, the subnet auto-discovery will not work.

|Flag                                   | Type                            | Default         | Description |
|---------------------------------------|---------------------------------|-----------------|-------------|
|aws-api-endpoints                      | AWS API Endpoints Config        |                 | AWS API endpoints mapping, format: serviceID1=URL1,serviceID2=URL2 |
|aws-api-throttle                       | AWS Throttle Config             | [default value](#default-throttle-config ) | throttle settings for AWS APIs, format: serviceID1:operationRegex1=rate:burst,serviceID2:operationRegex2=rate:burst |
|aws-max-retries                        | int                             | 10              | Maximum retries for AWS APIs |
|aws-region                             | string                          | [instance metadata](#instance-metadata)   | AWS Region for the kubernetes cluster |
|aws-vpc-id                             | string                          | [instance metadata](#instance-metadata)   | AWS VPC ID for the Kubernetes cluster |
|allowed-certificate-authority-arns     | stringList                      | []              | Specify an optional list of CA ARNs to filter on in cert discovery (empty means all CAs are allowed) |
|backend-security-group                 | string                          |                 | Backend security group id to use for the ingress rules on the worker node SG|
|cluster-name                           | string                          |                 | Kubernetes cluster name|
|default-ssl-policy                     | string                          | ELBSecurityPolicy-2016-08 | Default SSL Policy that will be applied to all Ingresses or Services that do not have the SSL Policy annotation |
|default-tags                           | stringMap                       |                 | AWS Tags that will be applied to all AWS resources managed by this controller. Specified Tags takes highest priority |
|default-target-type                    | string                          | instance        | Default target type for Ingresses and Services - ip, instance |
|[disable-ingress-class-annotation](#disable-ingress-class-annotation)       | boolean                         | false           | Disable new usage of the `kubernetes.io/ingress.class` annotation |
|[disable-ingress-group-name-annotation](#disable-ingress-group-name-annotation)  | boolean                         | false           | Disallow new use of the `alb.ingress.kubernetes.io/group.name` annotation |
|disable-restricted-sg-rules            | boolean                         | false           | Disable the usage of restricted security group rules |
|enable-backend-security-group          | boolean                         | true            | Enable sharing of security groups for backend traffic |
|enable-endpoint-slices                 | boolean                         | false           | Use EndpointSlices instead of Endpoints for pod endpoint and TargetGroupBinding resolution for load balancers with IP targets. |
|enable-leader-election                 | boolean                         | true            | Enable leader election for the load balancer controller manager. Enabling this will ensure there is only one active controller manager |
|enable-pod-readiness-gate-inject       | boolean                         | true            | If enabled, targetHealth readiness gate will get injected to the pod spec for the matching endpoint pods |
|enable-shield                          | boolean                         | true            | Enable Shield addon for ALB |
|[enable-waf](#waf-addons)                             | boolean                         | true            | Enable WAF addon for ALB |
|[enable-wafv2](#waf-addons)                           | boolean                         | true            | Enable WAF V2 addon for ALB |
|external-managed-tags                  | stringList                      |                 | AWS Tag keys that will be managed externally. Specified Tags are ignored during reconciliation |
|[feature-gates](#feature-gates)        | stringMap                       |                 | A set of key=value pairs to enable or disable features |
|health-probe-bind-addr                 | string                          | :61779          | The address the health probes binds to |
|ingress-class                          | string                          | alb             | Name of the ingress class this controller satisfies |
|ingress-max-concurrent-reconciles      | int                             | 3               | Maximum number of concurrently running reconcile loops for ingress |
|kubeconfig                             | string                          | in-cluster config | Path to the kubeconfig file containing authorization and API server information |
|leader-election-id                     | string                          | aws-load-balancer-controller-leader | Name of the leader election ID to use for this controller |
|leader-election-namespace              | string                          |                 | Name of the leader election ID to use for this controller |
|load-balancer-class                    | string                          | service.k8s.aws/nlb| Name of the load balancer class specified in service `spec.loadBalancerClass` reconciled by this controller |
|log-level                              | string                          | info            | Set the controller log level - info, debug |
|metrics-bind-addr                      | string                          | :8080           | The address the metric endpoint binds to |
|service-max-concurrent-reconciles      | int                             | 3               | Maximum number of concurrently running reconcile loops for service |
|[sync-period](#sync-period)                            | duration                        | 10h0m0s         | Period at which the controller forces the repopulation of its local object stores|
|targetgroupbinding-max-concurrent-reconciles | int                       | 3               | Maximum number of concurrently running reconcile loops for targetGroupBinding |
|targetgroupbinding-max-exponential-backoff-delay | duration              | 16m40s          | Maximum duration of exponential backoff for targetGroupBinding reconcile failures |
|tolerate-non-existent-backend-service  | boolean                         | true            | Whether to allow rules which refer to backend services that do not exist (When enabled, it will return 503 error if backend service not exist) |
|tolerate-non-existent-backend-action  | boolean                         | true            | Whether to allow rules which refer to backend actions that do not exist (When enabled, it will return 503 error if backend action not exist) |
|watch-namespace                        | string                          |                 | Namespace the controller watches for updates to Kubernetes objects, If empty, all namespaces are watched. |
|webhook-bind-port                      | int                             | 9443            | The TCP port the Webhook server binds to |
|webhook-cert-dir                       | string                          | /tmp/k8s-webhook-server/serving-certs | The directory that contains the server key and certificate |
|webhook-cert-file                      | string                          | tls.crt | The server certificate name |
|webhook-key-file                       | string                          | tls.key | The server key name |


### disable-ingress-class-annotation
`--disable-ingress-class-annotation` controls whether to disable new usage of the `kubernetes.io/ingress.class` annotation.

Once disabled:

* you can no longer create Ingresses with the value of the `kubernetes.io/ingress.class` annotation equal to `alb` (can be overridden via `--ingress-class` flag of this controller).

* you can no longer update Ingresses to set the value of the `kubernetes.io/ingress.class` annotation equal to `alb` (can be overridden via `--ingress-class` flag of this controller).

* you can still create Ingresses with a `kubernetes.io/ingress.class` annotation that has other values (for example: "nginx")

### disable-ingress-group-name-annotation
`--disable-ingress-group-name-annotation` controls whether to disable new usage of `alb.ingress.kubernetes.io/group.name` annotation.

Once disabled:

* you can no longer create Ingresses with the `alb.ingress.kubernetes.io/group.name` annotation.
* you can no longer alter the value of an `alb.ingress.kubernetes.io/group.name` annotation on an existing Ingress.

### sync-period
`--sync-period` defines a fixed interval for the controller to reconcile all resources even if there is no change, default to 10 hr. Please be mindful that frequent reconciliations may incur unnecessary AWS API usage.

As best practice, we do not recommend users to manually modify the resources managed by the controller. And users should not depend on the controller auto-reconciliation to revert the manual modification, or to mitigate any security risks.

### waf-addons
By default, the controller assumes sole ownership of the WAF addons associated to the provisioned ALBs, via the flag `--enable-waf` and `--enable-wafv2`.
And the users should disable them accordingly if they want a third party like AWS Firewall Manager to associate or remove the WAF-ACL of the ALBs.
Once disabled, the controller shall not take any actions on the waf addons of the provisioned ALBs.

### throttle config

Controller uses the following default throttle config:

```
WAF Regional:^AssociateWebACL|DisassociateWebACL=0.5:1,WAF Regional:^GetWebACLForResource|ListResourcesForWebACL=1:1,WAFV2:^AssociateWebACL|DisassociateWebACL=0.5:1,WAFV2:^GetWebACLForResource|ListResourcesForWebACL=1:1,Elastic Load Balancing v2:^RegisterTargets|^DeregisterTargets=4:20,Elastic Load Balancing v2:.*=10:40
```
Client side throttling enables gradual scaling of the api calls. Additional throttle config can be specified via the `--aws-api-throttle` flag. You can get the ServiceID from the API definition in AWS SDK. For e.g, ELBv2 it is [Elastic Load Balancing v2](https://github.com/aws/aws-sdk-go/blob/main/models/apis/elasticloadbalancingv2/2015-12-01/api-2.json#L9).

Here is an example of throttle config to specify client side throttling of ELBv2 calls.

```
--aws-api-throttle=Elastic Load Balancing v2:RegisterTargets|DeregisterTargets=4:20,Elastic Load Balancing v2:.*=10:40
```

### Instance metadata
If running on EC2, the default values are obtained from the instance metadata service.


### Feature Gates
They are a set of kye=value pairs that describe AWS load balance controller features. You can use it as flags `--feature-gates=key1=value1,key2=value2`

|Features-gate Supported Key            | Type                            | Default Value | Description                                                                                                                                                                          |
|---------------------------------------|---------------------------------|---------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| ListenerRulesTagging                  | string                          | true          | Enable or disable tagging AWS load balancer listeners and rules                                                                                                                      |
| WeightedTargetGroups                  | string                          | true          | Enable or disable weighted target groups                                                                                                                                             |
| ServiceTypeLoadBalancerOnly           | string                          | false         | If enabled, controller will be limited to reconciling service of type `LoadBalancer`                                                                                                 |
| EndpointsFailOpen                     | string                          | true          | Enable or disable allowing endpoints with `ready:unknown` state in the target groups.                                                                                                |
| EnableServiceController               | string                          | true          | Toggles support for `Service` type resources.                                                                                                                                        |
| EnableIPTargetType                    | string                          | true          | Used to toggle support for target-type `ip` across `Ingress` and `Service` type resources.                                                                                           |
| EnableRGTAPI                       | string                          | false         | If enabled, the tagging manager will describe resource tags via RGT APIs, otherwise via ELB APIs. In order to enable RGT API, `tag:GetResources` is needed in controller IAM policy. |
| SubnetsClusterTagCheck                | string                          | true          | Enable or disable the check for `kubernetes.io/cluster/${cluster-name}` during subnet auto-discovery                                                                                 |
| NLBHealthCheckAdvancedConfiguration   | string                          | true          | Enable or disable advanced health check configuration for NLB, for example health check timeout                                                                                      |
| ALBSingleSubnet                       | string                          | false         | If enabled, controller will allow using only 1 subnet for provisioning ALB, which need to get whitelisted by ELB in advance                                                          |
| NLBSecurityGroup                      | string                          | true          | Enable or disable all NLB security groups actions including frontend sg creation, backend sg creation, and backend sg modifications                                                  |
