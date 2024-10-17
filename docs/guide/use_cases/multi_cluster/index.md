# MultiCluster Target Groups

The LB controller assumes full control over configured Target Groups. This means when a Target Group is registered with the controller, it will deregister any targets that are not currently in the cluster. 
By enabling MultiCluster support, Target Groups can be associated to multiple Kubernetes clusters or support arbitrary targets from other sources.


## Overview

MultiCluster mode is supported in a couple different ways. Be sure that every cluster that is associated with a Target Group has one of these methods
enabled to ensure the Target Group is recognized as MultiCluster enabled! It is recommended to use fresh resources when using MutliCluster mode, there is short period of time
when the feature needs to generate a snapshot of the cluster state in order to support the mode. This data is stored into a ConfigMap which resides in the same namespace
as your load balancer resources. The ConfigMap stores the current snapshot of the managed targets, it can be found under `aws-lbc-targets-$TARGET_GROUP_BINDING_NAME`


ALB, specify this annotation in the ingress or service:

`alb.ingress.kubernetes.io/multi-cluster-target-group: "true"`

NLB, specify this annotation in your service:

`service.beta.kubernetes.io/aws-load-balancer-multi-cluster-target-group: "true"`


For Out-of-Band TargetGroupBindings, specify this field in the spec:

`multiClusterTargetGroup: true`



### Example

We will be setting up an echoserver in two clusters in order to demonstrate MultiCluster mode. See the full echoserver example in the 'Examples' tab.

The following ingress configures the Target Group Binding as MultiCluster. We will take the created Target Group and share it in a second cluster.

```
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: echoserver
  namespace: echoserver
  annotations:
    alb.ingress.kubernetes.io/multi-cluster-target-group: "true"    
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/tags: Environment=dev,Team=test
spec:
  ingressClassName: alb
  rules:
    - http:
        paths:
          - path: /
            pathType: Exact
            backend:
              service:
                name: echoserver
                port:
                  number: 80
```

Verify that MultiCluster is enabled by verifying that the created Target Group Binding is marked as MultiCluster.

```
kubectl -n echoserver get targetgroupbinding k8s-echoserv-echoserv-cc0122e143 -o yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: TargetGroupBinding
metadata:
  annotations:
    elbv2.k8s.aws/checkpoint: cKay81gadoTtBSg6uVVginqtmCVG-1ApTvYN4YLD37U/_4kBy3Yg64qrXzjvIb2LlC3O__ex1qjozynsqHXmPgo
    elbv2.k8s.aws/checkpoint-timestamp: "1729021572"
  creationTimestamp: "2024-10-15T19:46:06Z"
  finalizers:
  - elbv2.k8s.aws/resources
  generation: 1
  labels:
    ingress.k8s.aws/stack-name: echoserver
    ingress.k8s.aws/stack-namespace: echoserver
  name: k8s-echoserv-echoserv-cc0122e143
  namespace: echoserver
  resourceVersion: "79121011"
  uid: 9ceaa2ea-14bb-44a5-abb0-69c7d2aac52c
spec:
  ipAddressType: ipv4
  multiClusterTargetGroup: true <<< HERE
  networking:
    ingress:
    - from:
      - securityGroup:
          groupID: sg-06a2bd7d790ac1d2e
      ports:
      - port: 32197
        protocol: TCP
  serviceRef:
    name: echoserver
    port: 80
  targetGroupARN: arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-echoserv-cc0122e143/6816b87346280ee7
  targetType: instance
  vpcID: vpc-0a7ef5bd8943067a8
```

In another cluster, you can now register that Target Group ARN in a Target Group Binding.

```
apiVersion: elbv2.k8s.aws/v1beta1
kind: TargetGroupBinding
metadata:
  name: MyTargetGroupBinding
  namespace: echoserver
spec:
  serviceRef:
    name: echoserver
    port: 80
  multiClusterTargetGroup: true
  targetType: instance
  ipAddressType: ipv4
  networking:
    ingress:
    - from:
      - securityGroup:
          groupID: $SG_FROM_ABOVE
      ports:
      - port: 32197
        protocol: TCP
  targetGroupARN: $TG_FROM_ABOVE
```

The configured TargetGroup should have targets from both clusters available to service traffic.


