# Ingress specification
This document covers how ingress resources work in relation to The AWS Load Balancer Controller.

An example ingress, from [example](../../examples/2048/2048_full.yaml) is as follows.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: "2048-ingress"
  namespace: "2048-game"
  labels:
    app: 2048-nginx-ingress
spec:
  ingressClassName: alb
  rules:
    - host: 2048.example.com
      http:
        paths:
          - path: /*
            backend:
              service:
                name: "service-2048"
                port:
                  number: 80
```

The host field specifies the eventual Route 53-managed domain that will route to this service.

The service, service-2048, must be of type NodePort in order for the provisioned ALB to route to it.(see [echoserver-service.yaml](../../examples/echoservice/echoserver-service.yaml))

The AWS Load Balancer Controller does not support the `resource` field of `backend`.
