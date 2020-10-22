# Ingress specification
This document covers how ingress resources work in relation to The AWS Load Balancer Controller.

An example ingress, from [example](../../examples/2048/2048_full.yaml) is as follows.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: "2048-ingress"
  namespace: "2048-game"
  annotations:
    kubernetes.io/ingress.class: alb
  labels:
    app: 2048-nginx-ingress
spec:
  rules:
    - host: 2048.example.com
      http:
        paths:
          - path: /*
            backend:
              serviceName: "service-2048"
              servicePort: 80
```

The host field specifies the eventual Route 53-managed domain that will route to this service.

The service, service-2048, must be of type NodePort in order for the provisioned ALB to route to it.(see [echoserver-service.yaml](../../examples/echoservice/echoserver-service.yaml))

For details on purpose of annotations seen above, see [Annotations](annotations.md).
