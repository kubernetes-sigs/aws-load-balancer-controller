<p align="center">
    <img src="assets/images/kubernetes_icon.svg" alt="Kubernetes logo" width="200" />
    <img src="assets/images/aws_load_balancer_icon.svg" alt="AWS Load Balancer logo" width="200" />
</p>
<p align="center">
    <strong>
        A
        <a href="https://kubernetes.io/">Kubernetes </a>
        controller for
        <a href="https://aws.amazon.com/elasticloadbalancing/">Elastic Load Balancers</a>
    </strong>
</p>
<p align="center">
    <a href="https://github.com/kubernetes-sigs/aws-load-balancer-controller/issues">
        <img src="https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat" alt="contributions welcome"/>
    </a>
    <a href="https://github.com/kubernetes-sigs/aws-load-balancer-controller/issues">
        <img src="https://img.shields.io/github/issues-raw/kubernetes-sigs/aws-load-balancer-controller?style=flat" alt="github issues"/>
    </a>
    <img src="https://img.shields.io/badge/status-ga-brightgreen?style=flat" alt="status is ga"/>
    <img src="https://img.shields.io/github/license/kubernetes-sigs/aws-load-balancer-controller?style=flat" alt="apache license"/>
</p>
<p align="center">
    <a href="https://goreportcard.com/report/github.com/kubernetes-sigs/aws-load-balancer-controller">
        <img src="https://goreportcard.com/badge/github.com/kubernetes-sigs/aws-load-balancer-controller" alt="go report card"/>
    </a>
    <img src="https://img.shields.io/github/watchers/kubernetes-sigs/aws-load-balancer-controller?style=social" alt="github watchers"/>
    <img src="https://img.shields.io/github/stars/kubernetes-sigs/aws-load-balancer-controller?style=social" alt="github stars"/>
    <img src="https://img.shields.io/github/forks/kubernetes-sigs/aws-load-balancer-controller?style=social" alt="github forks"/>
    <a href="https://hub.docker.com/r/amazon/aws-alb-ingress-controller/">
        <img src="https://img.shields.io/docker/pulls/amazon/aws-alb-ingress-controller" alt="docker pulls"/>
    </a>
</p>


## AWS Load Balancer Controller

AWS Load Balancer Controller is a controller to help manage Elastic Load Balancers for a Kubernetes cluster.

  - It satisfies Kubernetes [Ingress resources](https://kubernetes.io/docs/concepts/services-networking/ingress/) by provisioning [Application Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/introduction.html).
  - It satisfies Kubernetes [Service resources](https://kubernetes.io/docs/concepts/services-networking/service/) by provisioning
[Network Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/introduction.html).

This project was formerly known as "AWS ALB Ingress Controller", we rebranded it to be "AWS Load Balancer Controller".

  - AWS ALB Ingress Controller was originated by [Ticketmaster](https://github.com/ticketmaster) and [CoreOS](https://github.com/coreos) as part of Ticketmaster's move to AWS and CoreOS Tectonic. Learn more about Ticketmaster's Kubernetes initiative from Justin Dean's video at [Tectonic Summit](https://www.youtube.com/watch?v=wqXVKneP0Hg).

  - AWS ALB Ingress Controller was donated to Kubernetes SIG-AWS to allow AWS, CoreOS, Ticketmaster and other SIG-AWS contributors to officially maintain the project. SIG-AWS reached this consensus on June 1, 2018.


## Security disclosures

If you think you’ve found a potential security issue, please do not post it in the Issues.  Instead, please follow the instructions [here](https://aws.amazon.com/security/vulnerability-reporting/) or [email AWS security directly](mailto:aws-security@amazon.com).

