# TargetGroupBinding
TargetGroupBinding is a [custom resource (CR)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) that can expose your pods using an existing [ALB TargetGroup](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html) or [NLB TargetGroup](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html).

This will allow you to provision the load balancer infrastructure completely outside of Kubernetes but still manage the targets with Kubernetes Service.

!!!tip "usage to support Ingress and Service"
    The AWS LoadBalancer controller internally used TargetGroupBinding to support the functionality for Ingress and Service resource as well.
    It automatically creates TargetGroupBinding in the same namespace of the Service used. 
    
    You can view all TargetGroupBindings in a namespace by `kubectl get targetgroupbindings -n <your-namespace> -o wide`


## TargetType
TargetGroupBinding CR supports TargetGroups of either `instance` or `ip` TargetType.

!!!tip ""
    If TargetType is not explicitly specified, a mutating webhook will automatically call AWS API to find the TargetType for your TargetGroup and set it to correct value.


## Sample YAML
```
apiVersion: elbv2.k8s.aws/v1beta1
kind: TargetGroupBinding
metadata:
  name: my-tgb
spec:
  serviceRef:
    name: awesome-service # route traffic to the awesome-service
    port: 80
  targetGroupARN: <arn-to-targetGroup>
```

## Reference
See the [reference](./spec.md) for TargetGroupBinding CR