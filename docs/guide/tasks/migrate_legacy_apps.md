# Migrating From Legacy Apps with Manually Configured Target Groups

Many organizations are decomposing old legacy apps into smaller services and components.

During the transition they may be running a hybrid ecosystem with some parts of the app running in ec2 instances,
some in Kubernetes microservices, and possibly even some in serverless environments like Lambda.

The existing clients of the application expect all endpoints under one DNS entry and it's desirable to be able
to route traffic at the ALB to services running outside the Kubernetes cluster.

The actions annotation allows the definition of a forward rule to a previously configured target group.
Learn more about the actions annotation at
[`alb.ingress.kubernetes.io/actions.${action-name}`](../ingress/annotations.md#actions)

## Example Ingress Manifest
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  namespace: testcase
  name: echoserver
  annotations:
    alb.ingress.kubernetes.io/actions.legacy-app: '{"Type": "forward", "TargetGroupArn": "legacy-tg-arn"}'
spec:
  ingressClassName: alb
  rules:
    - http:
        paths:
          - path: /v1/endpoints
            pathType: Exact
            backend:
              service:
                name: legacy-app
                port:
                  name: use-annotation
          - path: /normal-path
            pathType: Exact
            backend:
              service:
                name: echoserver
                port:
                  number: 80
```

!!!note
    The `TargetGroupArn` must be set and the user is responsible for configuring the Target group in AWS before applying
    the forward rule.

