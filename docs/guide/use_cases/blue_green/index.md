
You can configure an Application Load Balancer (ALB) to split traffic from the same listener across multiple target groups. This facilitates A/B testing, blue/green deployment, and traffic management without additional tools. More specifically, the AWS Application Load Balancer supports [weighted target groups]() and [advanced request routing](). 

The Load Balancer Controller (LBC) supports defining this behavior from Kubernetes, alongside the standard configuration of an Ingress resource. 

Weighted target group
Multiple target groups can be attached to the same forward action of a listener rule and specify a weight for each group. It allows developers to control how to distribute traffic to multiple versions of their application. For example, when you define a rule having two target groups with weights of 8 and 2, the load balancer will route 80 percent of the traffic to the first target group and 20 percent to the other.

Advanced request routing
In addition to the weighted target group, AWS announced the advanced request routing feature in 2019. Advanced request routing gives developers the ability to write rules (and route traffic) based on standard and custom HTTP headers and methods, the request path, the query string, and the source IP address. This new feature simplifies the application architecture by eliminating the need for a proxy fleet for routing, blocks unwanted traffic at the load balancer, and enables the implementation of A/B testing.

Example:

```
annotations:
   ...
  alb.ingress.kubernetes.io/actions.blue-green: |
    {
      "type":"forward",
      "forwardConfig":{
        "targetGroups":[
          {
            "serviceName":"hello-kubernetes-v1",
            "servicePort":"80",
            "weight":50
          },
          {
            "serviceName":"hello-kubernetes-v2",
            "servicePort":"80",
            "weight":50
          }
        ]
      }
    }
```

Walkthrough
Prerequisites
A good understanding of AWS Application Load Balancer, Amazon EKS, and Kubernetes.
The Amazon EKS command-line tool, eksctl.
The Kubernetes command-line tool, kubectl, and helm.
Create your EKS cluster with eksctl
Create your Amazon EKS cluster with the following command. You can replace cluster name dev with your own value. Replace region-code with any Region that is supported by Amazon EKS.
More details can be found from Getting started with Amazon EKS — eksctl.

Deploy ingress and test the blue/green deployment
Ingress annotation alb.ingress.kubernetes.io/actions.${action-name} provides a method for configuring custom actions on the listener of an Application Load Balancer, such as redirect action, forward action. With forward action, multiple target groups with different weights can be defined in the annotation. AWS Load Balancer Controller provisions the target groups and configures the listener rules as per the annotation to direct the traffic. For example, the following ingress resource configures the Application Load Balancer to forward all traffic to hello-kubernetes-v1 service (weight: 100 vs. 0).

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

**Note, the action-name in the annotation must match the serviceName in the ingress rules, and servicePort must be use-annotation as in the previous code snippet.**


