# ALB Ingress Controller

The ALB Ingress Controller satisfies Kubernetes [ingress resources](https://kubernetes.io/docs/user-guide/ingress) by provisioning an [Application Load Balancer](https://aws.amazon.com/elasticloadbalancing/applicationloadbalancer) and Route 53 DNS record set.

## Installation

The ALB container is installable via `kubectl` or `helm`. Follow one of the two options below which will create a [Kubernetes deployment](https://kubernetes.io/docs/user-guide/deployments). Only a single instance should be run at a time. Any issues, crashes, or other rescheduling needs will be handled by Kubernetes natively.

**[TODO]**: Need to validate iam-policy.json mentioned below and see if it can be refined.

To perform operations the controller must have requred IAM role capabilities to access and provision ALB and Route53 resources. There are many ways to achieve this, such as loading `AWS_ACCESS_KEY_ID`/`AWS_ACCESS_SECRET_KEY` as environment variables or using [kube2iam](https://github.com/jtblin/kube2iam). A sample IAM policy with the minimum permissions to run the controller can be found in [examples/alb-iam-policy.json](examples/iam-policy.json).

**[TODO]**: Need to verify ingress.class, mentioned below,  works OOTB with this controller. IF not, seems very valuable to implement.

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
helm registry install quay.io/coreos/alb-ingress
```

The controller will see ingress events for all namespaces in your cluster. Ingress resources that do not contain [necessary annotations](#annotations) will automatically be ignored. However, you may wish to limit the scope of ingress resources this controller has visibility into. In this case, you can define an `ingress.class` annotation, set the `--watch-namespace=` argument, or both.

Setting the `kubernetes.io/ingress.class: "alb"` annotation allows for classification of ingress resources and is especially helpful when running multiple ingress controllers in the same cluster. See [Using Multiple Ingress Controllers](https://github.com/nginxinc/kubernetes-ingress/tree/master/examples/multiple-ingress-controllers#using-multiple-ingress-controllers) for more details.

Setting the `--watch-namespace` argument constrains the ALB ingress-controller's scope to a **single** namespace. Ingress events outside of the namespace specified here will not be seen by the controller. Currently you cannot specify a watch on multiple namespaces or blacklist specific namespaces. See [this Kubernetes issue](https://github.com/kubernetes/contrib/issues/847) for more details.

Once configured as needed, the controller can be deployed like any Kubernetes deployment.

```bash
$ kubectl apply -f alb-ingress-controller.yaml
```

### Ingress Behavior

Periodically, ingress update events are seen by the controller. The controller retains a list of all ingress resources it knows about, along with the current state of AWS components that satisfy them. When an update event is fired, the controller re-scans the list of ingress resources known to the cluster and determines, by comparing the list to its previously stored one, the ingresses requiring deletion, creation or modification.

An example ingress, from `example/2048/2048-ingress.yaml` is as follows.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: "nginx-ingress"
  namespace: "2048-game"
  annotations:
    alb.ingress.kubernetes.io/scheme: internal
    alb.ingress.kubernetes.io/subnets: subnet-1234
    alb.ingress.kubernetes.io/security-groups: sg-1234
  labels:
    app: 2048-nginx-ingress
spec:
  rules:
  - host: 2048.example.com
    http:
      paths:
      - path: /
        backend:
          serviceName: "service-2048"
          servicePort: 80
```

The host field specifies the eventual Route 53-managed domain that will route to this service. The service, service-2048, must be of type NodePort (see [examples/echoservice/echoserver-service.yaml](examples/echoservice/echoserver-service.yaml)) in order for the provisioned ALB to route to it. If no NodePort exists, the controller will not attempt to provision resources in AWS. For details on purpose of annotations seen above, see [Annotations](#annotations).

## Annotations

The ALB Ingress Controller is configured by Annotations on the `Ingress` resource object. Some are required and some are optional.

### Required Annotations

```
alb.ingress.kubernetes.io/security-groups
alb.ingress.kubernetes.io/subnets
```

Required annotations use, the namespace is omitted for brevity.

- **security-groups**: Required. [Security groups](http://docs.aws.amazon.com/AmazonVPC/latest/UserGuide/VPC_SecurityGroups.html) that should be applied to the ALB instance. Example value: `subnet-a4f0098e,subnet-457ed533,subnet-95c904cd`

- **subnets**: Required. The subnets where the ALB instance should be deployed. Must include 2 subnets, each in a different [availability zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html). Example value: `sg-723a380a,sg-a6181ede,sg-a5181edd`

### Optional Annotations

```
alb.ingress.kubernetes.io/backend-protocol
alb.ingress.kubernetes.io/certificate-arn
alb.ingress.kubernetes.io/healthcheck-path
alb.ingress.kubernetes.io/port
alb.ingress.kubernetes.io/scheme
alb.ingress.kubernetes.io/successCodes
alb.ingress.kubernetes.io/tags
```

Optional annotations use, the namespace is omitted for brevity.

- **backend-protocol**: Enables selection of protocol for ALB to use to connect to backend service. When omitted, `HTTP` is used.

- **certificate-arn**: Enables HTTPS and uses the certificate defined, based on arn, stored in your [AWS Certificate Manager](https://aws.amazon.com/certificate-manager).

- **healthcheck-path**: Defines the path ALB health checks will occur. When omitted, `/` is used.

- **port**: Defines the port the ALB is exposed. When omitted, `80` is used for HTTP and `443` is used for HTTPS.

- **scheme**: Defines whether an ALB should be `internal` or `internet-facing`. See [Load balancer scheme] in the AWS documentation for more details.

- **successCodes**: Defines the HTTP status code that should be expected when doing health checks against the defined `healthcheck-path`. When omitted, `200` is used.

- **tags**: Defines [AWS Tags](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Tags.html) that should be applied to the ALB instance and Target groups.

## Building

For details on building this project, see [BUILDING.md](./BUILDING.md).
