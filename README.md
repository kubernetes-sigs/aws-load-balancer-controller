[![Coverage Status](https://coveralls.io/repos/github/coreos/alb-ingress-controller/badge.svg?branch=master)](https://coveralls.io/github/coreos/alb-ingress-controller?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/coreos/alb-ingress-controller)](https://goreportcard.com/report/github.com/coreos/alb-ingress-controller)
[![Build Status](https://travis-ci.org/coreos/alb-ingress-controller.svg?branch=master)](https://travis-ci.org/coreos/alb-ingress-controller)

# ALB Ingress Controller

The ALB Ingress Controller satisfies Kubernetes [ingress resources](https://kubernetes.io/docs/user-guide/ingress) by provisioning [Application Load Balancers](https://aws.amazon.com/elasticloadbalancing/applicationloadbalancer) and [Route 53 Resource Record Sets](http://docs.aws.amazon.com/Route53/latest/DeveloperGuide/rrsets-working-with.html).

This project was originated by [Ticketmaster](https://github.com/ticketmaster) and [CoreOS](https://github.com/coreos) as part of Ticketmaster's move to AWS and CoreOS Tectonic. Learn more about Ticketmaster's Kubernetes initiative from Justin Dean's video at [Tectonic Summit](https://www.youtube.com/watch?v=wqXVKneP0Hg).

## Design

The following diagram details the AWS components this controller creates. It also demonstrates the route ingress traffic takes from the DNS record to the Kubernetes cluster.

![controller-design](docs/imgs/controller-design.png)

### Ingress Creation

This section describes each step (circle) above. This example demonstrates satisfying 1 ingress resource.

**[1]**: The controller watches for [ingress
events](https://godoc.org/k8s.io/ingress/core/pkg/ingress#Controller) from the API server. When it
finds ingress resources that satisfy its requirements, it begins the creation of AWS resources.

**[2]**: An
[ALB](http://docs.aws.amazon.com/elasticbeanstalk/latest/dg/environments-cfg-applicationloadbalancer.html) (ELBv2) is created in AWS for the new ingress resource. This ALB can be internet-facing or internal. You can also specify the subnets its created in
using annotations.

**[3]**: [Target Groups](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html) are created in AWS for each unique Kubernetes service described in the ingress resource.

**[4]**: [Listeners](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html) are created for every port detailed in your ingress resource annotations. When no port is specified, sensible defaults (`80` or `443`) are used. Certificates may also be attached via annotations.

**[5]**: [Rules](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-update-rules.html) are created for each path specified in your ingress resource. This ensures traffic to a specific path is routed to the correct Kubernetes Service.

**[6]**: A [Route 53 Resource Record Set](http://docs.aws.amazon.com/Route53/latest/DeveloperGuide/rrsets-working-with.html) is created representing the domain of the ingress resource.

Along with the above, the controller also...

- deletes AWS components when ingress resources are removed from k8s.
- modifies AWS components when ingress resources change in k8s.
- assembles a list of existing ingress-related AWS components on start-up. Allowing you to
  recover if the controller were to be restarted.

### Ingress Traffic

This section details how traffic reaches the cluster.

As seen above, the ingress traffic for controller-managed resources starts at Route 53 DNS, moves
through the ALB, and reaches the Kubernetes nodes through each service's NodePort. This means that
services referenced from ingress resource must be exposed on a node port in order to be to be
reached by the ALB.

## Installation

The ALB Ingress Controller is installable via `kubectl` or `helm`. Follow one of the two options below to create a [Kubernetes deployment](https://kubernetes.io/docs/user-guide/deployments).

In order to start the controller, it must have appropriate access to AWS APIs. See [AWS API
Access](docs/configuration.md#aws-api-access) for more information.

### kubectl Install

Deploy a default backend service.
 
```
$ kubectl create -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/default-backend.yaml
```

> Specifying a `--default-backend-service=` is required by Kubernetes ingress controllers. However, the ALB Ingress Controller does not make use of this service.

Deploy the ALB Ingress Controller.

```  
kubectl create -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/alb-ingress-controller.yaml
```

> The `AWS_REGION` in the above example is set to `us-west-1`. Change this if your cluster is in a different region.

### Helm App Reqistry Install

You must have the [Helm App Registry plugin](https://coreos.com/apps) installed for these instructions to work.

```
helm registry install quay.io/coreos/alb-ingress-controller-helm
```

## Ingress Resources

Once the ALB Ingress Controller is running, you're ready to add ingress resources for it to satisfy.
Examples can be found in the [examples](examples) directory.

All ingress resources deployed must specify security groups and subnets for the provisioned ALB to
use. See the [Annotations](docs/ingress-resources.md#annotations) section of the documentation to understand how this is configured.

## Documentation

Visit the [docs](docs/) directory for documentation on usage and configuration of the ALB Ingress
Controller.

## Building

For details on building this project, see [BUILDING.md](./BUILDING.md).
