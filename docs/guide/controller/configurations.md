# AWS Load Balancer controller configuration options
## Controller command line flags

!!!warning ""
    The --cluster-name flag is mandatory and the value must match the name of the kubernetes cluster. If you specify an incorrect name, the subnet auto-discovery will not work.

|Flag                                   | Type                            | Default         | Description |
|---------------------------------------|---------------------------------|-----------------|-------------|
|aws-api-throttle                       | AWS Throttle Config             | [default value](#Default throttle config ) | throttle settings for AWS APIs, format: serviceID1:operationRegex1=rate:burst,serviceID2:operationRegex2=rate:burst |
|aws-max-retries                        | int                             | 10              | Maximum retries for AWS APIs |
|aws-region                             | string                          | [instance metadata](#Instance metadata)    | AWS Region for the kubernetes cluster |
|aws-vpc-id                             | string                          | [instance metadata](#Instance metadata)    | AWS VPC ID for the Kubernetes cluster |
|cluster-name                           | string                          |                 | Kubernetes cluster name|
|enable-leader-election                 | boolean                         | true            | Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager. |
|enable-pod-readiness-gate-inject       | boolean                         | true            | If enabled, targetHealth readiness gate will get injected to the pod spec for the matching endpoint pods. |
|enable-shield                          | boolean                         | true            | Enable Shield addon for ALB |
|enable-waf                             | boolean                         | true            | Enable WAF addon for ALB |
|enable-wafv2                           | boolean                         | true            | Enable WAF V2 addon for ALB |
|ingress-class                          | string                          |                 | Name of the ingress class this controller satisfies |
|ingress-max-concurrent-reconciles      | int                             | 3               | Maximum number of concurrently running reconcile loops for ingress |
|kubeconfig                             | string                          | in-cluster config | Path to the kubeconfig file containing authorization and API server information |
|leader-election-id                     | string                          | aws-load-balancer-controller-leader | Name of the leader election ID to use for this controller |
|leader-election-namespace              | string                          |                 | Name of the leader election ID to use for this controller |
|log-level                              | string                          | info            | Set the controller log level - info, debug |
|metrics-bind-addr                      | string                          | :8080           | The address the metric endpoint binds to |
|service-max-concurrent-reconciles      | int                             | 3               | Maximum number of concurrently running reconcile loops for service |
|sync-period                            | duration                        | 1h0m0s          | Period at which the controller forces the repopulation of its local object stores|
|targetgroupbinding-max-concurrent-reconciles | int                       | 3               | Maximum number of concurrently running reconcile loops for targetGroupBinding |
|watch-namespace                        | string                          |                 | Namespace the controller watches for updates to Kubernetes objects, If empty, all namespaces are watched. |
|webhook-bind-port                      | int                             | 9443            | The TCP port the Webhook server binds to |


### Default throttle config
```
WAF Regional:^AssociateWebACL|DisassociateWebACL=0.5:1,WAF Regional:^GetWebACLForResource|ListResourcesForWebACL=1:1,WAFV2:^AssociateWebACL|DisassociateWebACL=0.5:1,WAFV2:^GetWebACLForResource|ListResourcesForWebACL=1:1
```

### Instance metadata
If running on EC2, the default values are obtained from the instance metadata service.
