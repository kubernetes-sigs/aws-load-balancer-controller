# Split Traffic

You can configure an Application Load Balancer (ALB) to split traffic from the same listener across multiple target groups using rules. This facilitates A/B testing, blue/green deployment, and traffic management without additional tools. The Load Balancer Controller (LBC) supports defining this behavior alongside the standard configuration of an Ingress resource. 

More specifically, the ALB supports weighted target groups and advanced request routing. 

**Weighted target group**
Multiple target groups can be attached to the same forward action of a listener rule and specify a weight for each group. It allows developers to control how to distribute traffic to multiple versions of their application. For example, when you define a rule having two target groups with weights of 8 and 2, the load balancer will route 80 percent of the traffic to the first target group and 20 percent to the other.

**Advanced request routing**
In addition to the weighted target group, AWS announced the advanced request routing feature in 2019. Advanced request routing gives developers the ability to write rules (and route traffic) based on standard and custom HTTP headers and methods, the request path, the query string, and the source IP address. This new feature simplifies the application architecture by eliminating the need for a proxy fleet for routing, blocks unwanted traffic at the load balancer, and enables the implementation of A/B testing.

## Overview
The ALB is configured to split traffic using annotations on the ingress resrouces. More specifically, the [ingress annotation](../../../guide/ingress/annotations.md#actions) `alb.ingress.kubernetes.io/actions.${service-name}` configures custom actions on the listener. 

The body of the annotation is a JSON document that identifies an action type, and configures it. The supported [actions](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html#rule-action-types) are `redirect`, `forward`, and `fixed-response`. 

With forward action, multiple target groups with different weights can be defined in the annotation. The LBC provisions the target groups and configures the listener rules as per the annotation to direct the traffic. 

Importantly: 
* The `action-name` in the annotation must match the service name in the Ingress rules. For example, the annotation `alb.ingress.kubernetes.io/actions.blue-green` matches the service name `blue-green` referenced in the Ingress rules. 
* The `servicePort` of the service in the Ingress rules must be `use-annotation`.

### Example

The following ingress resource configures the ALB to forward all traffic to hello-kubernetes-v1 service (weight: 100 vs. 0).

Note that the annotation name includes `blue-green`, which matches the service name referenced in the ingress rules. 

The [annotation reference](../../../guide/ingress/annotations.md#actions) includes further examples of the JSON configuration for different actions.

```
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: "hello-kubernetes"
  namespace: "hello-kubernetes"
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/actions.blue-green: |
      {
        "type":"forward",
        "forwardConfig":{
          "targetGroups":[
            {
              "serviceName":"hello-kubernetes-v1",
              "servicePort":"80",
              "weight":100
            },
            {
              "serviceName":"hello-kubernetes-v2",
              "servicePort":"80",
              "weight":0
            }
          ]
        }
      }
  labels:
    app: hello-kubernetes
spec:
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: blue-green
                port:
                  name: use-annotation
```



