# URL rewrite

URL rewrite enables request URLs to be transformed before the request reaches your backend services.

Consider the following scenario:

- An ingress exposes two services:
  - An API service exposed at path prefix `/api/`.
  - A users service exposed at path prefix `/users/`.
- These service **do not expect** the `/api/` or `/users/` prefixes in the request path.

For this scenario, URL rewrite would be used to remove the leading `/api/` and `/users/` from the request paths.

Example ingress manifest:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  namespace: default
  name: ingress
  annotations:
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-west-2:xxxx:certificate/xxxxxx
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS":443}]'
    alb.ingress.kubernetes.io/ssl-redirect: "443"

    # Add transform to users-service to remove "/users/" prefix from paths
    alb.ingress.kubernetes.io/transforms.users-service: >
      [
        {
          "type": "url-rewrite",
          "urlRewriteConfig": {
            "rewrites": [{
              "regex": "^/users/(.+)$",
              "replace": "/$1"
            }]
          }
        }
      ]
    # Add transform to api-service to remove "/api/" prefix from paths
    alb.ingress.kubernetes.io/transforms.api-service: >
      [
        {
          "type": "url-rewrite",
          "urlRewriteConfig": {
            "rewrites": [{
              "regex": "^/api/(.+)$",
              "replace": "/$1"
            }]
          }
        }
      ]
spec:
  ingressClassName: alb
  rules:
    - http:
        paths:
          - path: /users/
            pathType: Prefix
            backend:
              service:
                name: users-service
                port:
                  number: 80
          - path: /api/
            pathType: Prefix
            backend:
              service:
                name: api-service
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

## Host header rewrite

To rewrite from `api.example.com` to `example.com`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  namespace: default
  name: ingress
  annotations:
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-west-2:xxxx:certificate/xxxxxx
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS":443}]'
    alb.ingress.kubernetes.io/ssl-redirect: "443"

    # add transform to api-service to replace "api.example.com" hostname with "example.com"
    alb.ingress.kubernetes.io/transforms.api-service: >
      [
        {
          "type": "host-header-rewrite",
          "hostHeaderRewriteConfig": {
            "rewrites": [{
              "regex": "^api\\.example\\.com$",
              "replace": "example.com"
            }]
          }
        }
      ]
spec:
  ingressClassName: alb
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /*
            pathType: Prefix
            backend:
              service:
                name: api-service
                port:
                  number: 80
```

## Additional resources

- [`alb.ingress.kubernetes.io/transforms.${transforms-name}` annotation documentation](../../ingress/annotations#transforms)
