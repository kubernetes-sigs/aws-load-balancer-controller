# Configuration

This document covers configuration of the ALB Ingress Controller.

## AWS API Access

To perform operations, the controller must have requred IAM role capabilities for accessing and
provisioning ALB and Route53 resources. There are many ways to achieve this, such as loading `AWS_ACCESS_KEY_ID/AWS_ACCESS_SECRET_KEY` as environment variables or using [kube2iam](https://github.com/jtblin/kube2iam). 

A sample IAM policy, with the minimum permissions to run the controller, can be found in [examples/alb-iam-policy.json](../examples/iam-policy.json).  

## Setting Ingress Resource Scope

By default, all ingress resources in your cluster are seen by the controller. However, only ingress resources that contain the [required annotations](#annotations) will be satisfied by the ALB Ingress Controller. 

You can further limit the ingresses your controller has access to. The options available are limiting the ingress class  (`ingress.class`) or limiting the namespace watched (`--watch-namespace=`). Each approach is detailed below.

### Limiting Ingress Class

_[This feature is currently being implemented]_

Setting the `kubernetes.io/ingress.class` annotation allows for classification of ingress resources and is especially helpful when running multiple ingress controllers in the same cluster. See [Using Multiple Ingress Controllers](https://github.com/nginxinc/kubernetes-ingress/tree/master/examples/multiple-ingress-controllers#using-multiple-ingress-controllers) for more details.

### Limiting Namespaces

Setting the `--watch-namespace` argument constrains the controller's scope to a single namespace. Ingress events outside of the namespace specified are not be seen by the controller. An example of the container spec, for a controller watching only the `default` namespace, is as follows.

```yaml
spec:
  containers:
  - args:
    - /server
    - --default-backend-service=kube-system/default-http-backend
    - --watch-namespace=default
```

> Currently, you can set only 1 namespace to watch in this flag. See [this Kubernetes issue](https://github.com/kubernetes/contrib/issues/847) for more details.
