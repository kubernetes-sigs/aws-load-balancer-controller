[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/aws-alb-ingress-controller/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-sigs/aws-alb-ingress-controller?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-sigs/aws-alb-ingress-controller)](https://goreportcard.com/report/github.com/kubernetes-sigs/aws-alb-ingress-controller)
[![Build Status](https://travis-ci.org/kubernetes-sigs/aws-alb-ingress-controller.svg?branch=master)](https://travis-ci.org/kubernetes-sigs/aws-alb-ingress-controller)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcoreos%2Falb-ingress-controller.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcoreos%2Falb-ingress-controller?ref=badge_shield)

# AWS ALB Ingress Controller

**NOTE:** This controller is in alpha state as we attempt to move to our first 1.0 release. The current image version is `1.0-alpha.9`. Please file any issues you find and note the version used.

The AWS ALB Ingress Controller satisfies Kubernetes [ingress resources](https://kubernetes.io/docs/user-guide/ingress) by provisioning [Application Load Balancers](https://aws.amazon.com/elasticloadbalancing/applicationloadbalancer).

This project was originated by [Ticketmaster](https://github.com/ticketmaster) and [CoreOS](https://github.com/coreos) as part of Ticketmaster's move to AWS and CoreOS Tectonic. Learn more about Ticketmaster's Kubernetes initiative from Justin Dean's video at [Tectonic Summit](https://www.youtube.com/watch?v=wqXVKneP0Hg).

This project was donated to Kubernetes SIG-AWS to allow AWS, CoreOS, Ticketmaster and other SIG-AWS contributors to officially maintain the project. SIG-AWS reached this consensus on June 1, 2018.

## Getting started

To get started with the controller, see our [walkthrough](docs/walkthrough.md).

## Design

The following diagram details the AWS components this controller creates. It also demonstrates the route ingress traffic takes from the ALB to the Kubernetes cluster.

![controller-design](docs/imgs/controller-design.png)

### Ingress Creation

This section describes each step (circle) above. This example demonstrates satisfying 1 ingress resource.

**[1]**: The controller watches for [ingress
events](https://godoc.org/k8s.io/ingress/core/pkg/ingress#Controller) from the API server. When it
finds ingress resources that satisfy its requirements, it begins the creation of AWS resources.

**[2]**: An
[ALB](http://docs.aws.amazon.com/elasticbeanstalk/latest/dg/environments-cfg-applicationloadbalancer.html) (ELBv2) is created in AWS for the new ingress resource. This ALB can be internet-facing or internal. You can also specify the subnets it's created in
using annotations.

**[3]**: [Target Groups](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html) are created in AWS for each unique Kubernetes service described in the ingress resource.

**[4]**: [Listeners](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html) are created for every port detailed in your ingress resource annotations. When no port is specified, sensible defaults (`80` or `443`) are used. Certificates may also be attached via annotations.

**[5]**: [Rules](http://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-update-rules.html) are created for each path specified in your ingress resource. This ensures traffic to a specific path is routed to the correct Kubernetes Service.

Along with the above, the controller also...

- deletes AWS components when ingress resources are removed from k8s.
- modifies AWS components when ingress resources change in k8s.
- assembles a list of existing ingress-related AWS components on start-up, allowing you to
  recover if the controller were to be restarted.

### Ingress Traffic

This section details how traffic reaches the cluster.

As seen above, the ingress traffic for controller-managed resources starts at the ALB and reaches the Kubernetes nodes through each service's NodePort. This means that
services referenced from ingress resource must be exposed on a node port in order to be
reached by the ALB.

## Setup

For details on how to setup the controller and [external-dns](https://github.com/kubernetes-incubator/external-dns) (for managing Route 53), see [setup](docs/setup.md).

### Helm App Registry Install

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

## License
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcoreos%2Falb-ingress-controller.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcoreos%2Falb-ingress-controller?ref=badge_large)
