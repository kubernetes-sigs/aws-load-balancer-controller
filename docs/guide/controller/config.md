# ALB Ingress Controller Configuration
This document covers configuration of the ALB ingress controller

## AWS API Access
To perform operations, the controller must have required IAM role capabilities for accessing and
provisioning ALB resources. There are many ways to achieve this, such as loading `AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY` as environment variables or using [kube2iam](https://github.com/jtblin/kube2iam).

A sample IAM policy, with the minimum permissions to run the controller, can be found in [alb-iam-policy.json](../../examples/iam-policy.json).

## Setting Ingress Resource Scope
You can limit the ingresses ALB ingress controller controls by combining following two approaches:

### Limiting ingress class
Setting the `--ingress-class` argument constrains the controller's scope to ingresses with matching `kubernetes.io/ingress.class` annotation.
This is especially helpful when running multiple ingress controllers in the same cluster. See [Using Multiple Ingress Controllers](https://github.com/nginxinc/kubernetes-ingress/tree/master/examples/multiple-ingress-controllers#using-multiple-ingress-controllers) for more details.

An example of the container spec portion of the controller, only listening for resources with the class "alb", would be as follows.

```yaml
spec:
  containers:
  - args:
    - --ingress-class=alb
```

Now, only ingress resources with the appropriate annotation are picked up, as seen below.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: echoserver
  namespace: echoserver
  annotations:
    kubernetes.io/ingress.class: "alb"
spec:
    ...
```

### Limiting Namespaces
Setting the `--watch-namespace` argument constrains the controller's scope to a single namespace. Ingress events outside of the namespace specified are not be seen by the controller. 

An example of the container spec, for a controller watching only the `default` namespace, is as follows.

```yaml
spec:
  containers:
  - args:
    - --watch-namespace=default
```

> Currently, you can set only 1 namespace to watch in this flag. See [this Kubernetes issue](https://github.com/kubernetes/contrib/issues/847) for more details.

## Limiting External Namespaces

Setting the `--restrict-scheme` boolean flag to `true` will enable the ALB controller to check the configmap named `alb-ingress-controller-internet-facing-ingresses` for a list of approved ingresses before provisioning ALBs with an internet-facing scheme. Here is an example of that ConfigMap:

```yaml
apiVersion: v1
data:
 mynamespace: my-ingress-name, my-ingress-name-2
 myothernamespace: my-other-ingress-name
kind: ConfigMap
metadata:
  name: alb-ingress-controller-internet-facing-ingresses
```

This ConfigMap is kept in `default` if unspecified, and can be overridden via the `--restrict-scheme-namespace` flag.

## Resource Tags

Setting the `--default-tags` argument adds arbitrary tags to ALBs and target groups managed by the ingress controller.

```yaml
spec:
  containers:
  - args:
    - /server
    - --default-tags=mykey=myvalue,otherkey=othervalue
```    

## Subnet Auto Discovery
You can tag AWS subnets to allow ingress controller auto discover subnets used for ALBs.

- `kubernetes.io/cluster/${cluster-name}` must be set to `owned` or `shared`. Remember `${cluster-name}` needs to be the same name you're passing to the controller in the `--cluster-name` option

- `kubernetes.io/role/internal-elb` must be set to `1` or `` for internal LoadBalancers

- `kubernetes.io/role/elb` must be set to `1` or `` for internet-facing LoadBalancers

An example of a subnet with the correct tags for the cluster `joshcalico` is as follows:
![subnet-tags](../../imgs/subnet-tags.png)
