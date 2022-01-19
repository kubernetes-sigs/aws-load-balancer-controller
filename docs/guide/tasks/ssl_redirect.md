# Redirect Traffic from HTTP to HTTPS

You can use the [`alb.ingress.kubernetes.io/ssl-redirect`](../ingress/annotations.md#ssl-redirect) annotation to setup an ingress to redirect http traffic to https


## Example Ingress Manifest
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  namespace: default
  name: ingress
  annotations:
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-west-2:xxxx:certificate/xxxxxx
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS":443}]'
    alb.ingress.kubernetes.io/ssl-redirect: '443'
spec:
  ingressClassName: alb
  rules:
    - http:
        paths:
         - path: /users/*
           pathType: ImplementationSpecific
           backend:
             service:
               name: user-service
               port:
                 number: 80
         - path: /*
           pathType: ImplementationSpecific
           backend:
             service:
               name: default-service
               port:
                 number: 80
```

!!!note
    - `alb.ingress.kubernetes.io/listen-ports` annotation must at least include [{"HTTP": 80}, {"HTTPS":443}] to listen on 80 and 443.
    - `alb.ingress.kubernetes.io/certificate-arn` annotation must be set to allow listen for HTTPS traffic
    - the ssl-redirect port must appear in the listen-port annotation, and must be an HTTPS port

## How it works
If you enable SSL redirection, the controller configures each HTTP listener with a default action to redirect to HTTPS. The controller does not add any other rules to the HTTP listener.

For the above example, the HTTP listener on port 80 will have a single default rule to redirect traffic to HTTPS on port 443.
