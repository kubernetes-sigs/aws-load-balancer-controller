<p>
    <img src="assets/images/aws_load_balancer_icon.svg" alt="AWS LoadBalancer Logo" width="200" />
</p>

## AWS LoadBalancer Controller

AWS LoadBalancer Controller is a controller to help manage Elastic Load Balancers for a Kubernetes cluster.

  - It satisfies Kubernetes [Ingress resources](https://kubernetes.io/docs/concepts/services-networking/ingress/) by provisioning [Application Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/introduction.html).
  - It satisfies Kubernetes [Service resources](https://kubernetes.io/docs/concepts/services-networking/service/) by provisioning
[Network Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/introduction.html).

This project was priorly known as "AWS ALB Ingress Controller", we rebranded it to be "AWS LoadBalancer Controller".

  - AWS ALB Ingress Controller was originated by [Ticketmaster](https://github.com/ticketmaster) and [CoreOS](https://github.com/coreos) as part of Ticketmaster's move to AWS and CoreOS Tectonic. Learn more about Ticketmaster's Kubernetes initiative from Justin Dean's video at [Tectonic Summit](https://www.youtube.com/watch?v=wqXVKneP0Hg).

  - AWS ALB Ingress Controller was donated to Kubernetes SIG-AWS to allow AWS, CoreOS, Ticketmaster and other SIG-AWS contributors to officially maintain the project. SIG-AWS reached this consensus on June 1, 2018.


## Security disclosures

If you think you’ve found a potential security issue, please do not post it in the Issues.  Instead, please follow the instructions [here](https://aws.amazon.com/security/vulnerability-reporting/) or [email AWS security directly](mailto:aws-security@amazon.com).

## Documentation
Checkout our [Live Docs](https://kubernetes-sigs.github.io/aws-alb-ingress-controller/)!