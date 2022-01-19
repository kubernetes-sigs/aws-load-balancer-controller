# Certificate Discovery
TLS certificates for ALB Listeners can be automatically discovered with hostnames from Ingress resources if the [`alb.ingress.kubernetes.io/certificate-arn`](annotations.md#certificate-arn) annotation is not specified.

The controller will attempt to discover TLS certificates from the `tls` field in Ingress and `host` field in Ingress rules.

!!!note ""
    You need to explicitly specify to use HTTPS listener with [listen-ports](annotations.md#listen-ports) annotation.

## Discover via Ingress tls

!!!example
        - attaches certs for `www.example.com` to the ALB
            ```yaml
            apiVersion: networking.k8s.io/v1
            kind: Ingress
            metadata:
            namespace: default
            name: ingress
            annotations:
              alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS":443}]'
            spec:
              ingressClassName: alb
              tls:
              - hosts:
                - www.example.com
              rules:
              - http:
                  paths:
                  - path: /users
                    pathType: Prefix
                    backend:
                      service:
                        name: user-service
                        port:
                          number: 80
            ```


## Discover via Ingress rule host.

!!!example
        - attaches a cert for `dev.example.com` or `*.example.com` to the ALB
            ```yaml
            apiVersion: networking.k8s.io/v1
            kind: Ingress
            metadata:
            namespace: default
            name: ingress
            annotations:
              alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS":443}]'
            spec:
              ingressClassName: alb
              rules:
              - host: dev.example.com
                http:
                  paths:
                  - path: /users
                    pathType: Prefix
                    backend:
                      service:
                        name: user-service
                        port:
                          number: 80
            ```
