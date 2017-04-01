[![build status](http://git.tmaws.io/kubernetes/alb-ingress/badges/master/build.svg)](http://git.tmaws.io/kubernetes/alb-ingress/commits/master) [![coverage report](http://git.tmaws.io/kubernetes/alb-ingress/badges/master/coverage.svg)](http://git.tmaws.io/kubernetes/alb-ingress/commits/master)


# ALB Ingress Controller

The ALB ingress controller satisfies Kubernetes [ingress resources](https://kubernetes.io/docs/user-guide/ingress) by provisioning an [Application Load Balancer](https://aws.amazon.com/elasticloadbalancing/applicationloadbalancer) and Route 53 DNS record set.


## Installation

The ALB container is installable via `kubectl` or `helm`. Follow one of the two options below.

### kubectl Install

```
kubectl create -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/manifests/alb-ingress-controller.yaml
```

Optionally you can install a default backend to handle 404 pages:

```
kubectl create -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/manifests/default-backend.yaml
```

### Helm App Reqistry Install

NOTE: you must have the [Helm App Registry plugin](https://coreos.com/apps) installed for these instructions to work.

```
helm registry install quay.io/coreos/
```

## Annotations

The following annotations, when added to an ingress resource, are respected by the ALB Ingress Controller.

```
alb.ingress.kubernetes.io/backend-protocol
alb.ingress.kubernetes.io/certificate-arn
alb.ingress.kubernetes.io/healthcheck-path
alb.ingress.kubernetes.io/port
alb.ingress.kubernetes.io/scheme
alb.ingress.kubernetes.io/security-groups
alb.ingress.kubernetes.io/subnets
alb.ingress.kubernetes.io/successCodes
alb.ingress.kubernetes.io/tags
```

The following describes each annotations use, namespaces are omitted for brevity.

- **backend-protocol**: Optional. Enables selection of protocol for ALB to use to connect to backend service. When omitted, `HTTP` is used.

- **certificate-arn**: Optional. Enables HTTPS and uses the certificate defined, based on arn, stored in your [AWS Certificate Manager](https://aws.amazon.com/certificate-manager).

- **healthcheck-path**: Optional. Defines the path ALB health checks will occur. When omitted, `/` is used.

- **port**: Optional. Defines the port the ALB is exposed. When omitted, `80` is used for HTTP and `443` is used for HTTPS.

- **scheme**: Required. Defines whether an ALB should be `internal` or `internet-facing`. See [Load balancer scheme] in the AWS documentation for more details.

- **security-groups**: Required. [Security groups](http://docs.aws.amazon.com/AmazonVPC/latest/UserGuide/VPC_SecurityGroups.html) that should be applied to the ALB instance.

- **subnets**: Required. The subnets where the ALB instance should be deployed. Must include 2 subnets, each in a different [availability zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html).

- **successCodes**: Optional. Defines the HTTP status code that should be expected when doing health checks against the defined `healthcheck-path`. When omitted, `200` is used.

- **Tags**: Optional. Defines [AWS Tags](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Tags.html) that should be applied to the ALB instance and Target groups.

## Building

For details on building this project, see [BUILDING.md](./BUILDING.md).
