---
title : Legacy App Integration
---

## Motivation

Organizations may need a unified load balancing approach to maintain operational simplicity and cost efficiency while gradually migrating from legacy Amazon EC2 based applications to modern Amazon EKS microservices. This single load balancer strategy enables seamless user experience and supports phased modernization without disrupting existing services during digital transformation initiatives. The AWS Load Balancer Controller provides two approaches to achieve this unified load balancing architecture with an Application Load Balancer (ALB). This document dives into the implementation details of the second approach.

### Approach 1 : [Externally Managed Load Balancer](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/use_cases/self_managed_lb/)

In this approach, there are no Ingress objects configured on the EKS cluster. Instead:

1. Configure all forwarding rules and target groups directly on the AWS ALB using preferred tool (AWS SDK, CDK, API, CloudFormation, Terraform, etc.)
2. Configure TargetGroupBinding objects on the EKS cluster to associate each Kubernetes Service with a Target Group on the AWS ALB
3. The AWS Load Balancer Controller continuously tracks TargetGroupBindings and updates the target groups as pods change

### Approach 2 : `actions` annotation

In this approach,

1. Configure Ingress objects on the EKS cluster
2. Configure target groups for legacy applications on the ALB
3. Use the [alb.ingress.kubernetes.io/actions.${action-name}](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/ingress/annotations/#actions) annotation that associates the specific ingress rules with the target groups configured for legacy applications
4. When the Ingress object is deleted, the controller automatically deletes the associated target group(s)

## Prerequisites

- An EKS Cluster with [AWS Load Balancer Controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/deploy/installation/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/)
- A few EC2 instances to simulate a legacy application. 
- A sample application on the EKS cluster which is exposed through a Kubernetes service 

## Example

1. Create an Ingress object for the sample application. Sample manifest shown below.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name : myingress
  annotations:
    alb.ingress.kubernetes.io/load-balancer-name: myalb
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
spec:
  ingressClassName: alb
  rules:
  - http:
      paths:
      - backend:
          service:
            name: sampleservice
            port:
              number: 80
        path: /sampleapp
        pathType: Prefix
```

In this example the sample application on the EKS cluster is exposed through a Kubernetes service type of `ClusterIP` with the name `sampleservice`.

2. Configure a Target Group for Legacy Application

Navigate to the AWS Console and locate the ALB created by the AWS Load Balancer Controller. Create a new target group for the legacy application. Add the EC2 instances as targets. Copy the Amazon Resource Name (ARN) of the target group.

3. Update the Ingress object to include routing rules for the legacy application

Use `kubectl edit ingress myingress` or any other method to apply these changes.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    alb.ingress.kubernetes.io/actions.ec2: |
      {
        "type":"forward",
        "targetGroupARN": "YOUR_TARGET_GROUP_ARN_HERE"
      }
spec:
  rules:
  - http:
      paths:
      - backend:
          service:
            name: ec2
            port:
              name: use-annotation
        path: /legacy
        pathType: Exact
```

The `action-name` in the annotation must match the serviceName in the Ingress rules, and servicePort must be `use-annotation`. In this example the action-name of `ec2` matches the serviceName `ec2`. 

### Testing

From a client machine, test access to all applications using the ALB DNS name:

- **Legacy application**: `curl http://ALB_DNS_NAME/legacy`
- **Microservice**: `curl http://ALB_DNS_NAME/sampleapp`  


## Considerations

- Review ALB service quotas to ensure your architecture fits within limits
- Use the `group.name` annotation to group multiple Ingress objects on the same ALB
- Use the `group.order` annotation to prioritize Ingress objects in the ALB rules list for better performance
- When deleting Ingress objects, the controller will automatically clean up associated target groups

## References

- [TargetGroupBinding Documentation](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.13/guide/targetgroupbinding/targetgroupbinding/)
- [Ingress Actions Annotation](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.13/guide/ingress/annotations/#actions)
- [ALB Listener Rules](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-rules.html)
- [ALB Service Quotas](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-limits.html)
- [Group Name Annotation](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.13/guide/ingress/annotations/#group.name)
- [Group Order Annotation](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.13/guide/ingress/annotations/#group.order)
