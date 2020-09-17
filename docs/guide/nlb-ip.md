# NLB IP mode
AWS Load Balancer Controller supports Network Load Balancer (NLB) with IP targets for pods running on Amazon EC2 instances and AWS Fargate through Kubernetes service of type `LoadBalancer` with proper annotation. In this mode, the AWS NLB targets traffic directly to the Kubernetes pods behind the service, eliminating the need for an extra network hop through the worker nodes in the Kubernetes cluster.

## Prerequisites
* Kubernetes 1.20 and later with the AWS cloudprovider fix to allow external management of load balancers
* AWS Loadbalancer Controller v2.0
* Pods have native AWS VPC networking configured, see [Amazon VPC CNI plugin](https://github.com/aws/amazon-vpc-cni-k8s)

## Configuration
The NLB IP mode is determined based on the annotations added to the service object. For NLB in IP mode, apply the following annotation to the service:
```yaml
    metadata:
      name: my-service
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb-ip"
```

**NOTE**: Do not modify the service annotation `service.beta.kubernetes.io/aws-load-balancer-type` on an existing service object. If you need to modify the underlying AWS LoadBalancer type, for example from classic to NLB, delete the kubernetes service first and create again with the correct annotation. Failure to do so will result in leaked AWS load balancer resources.

The default load balancer is internet-facing. To create an internal load balancer, apply the following annotation to your service:

```
service.beta.kubernetes.io/aws-load-balancer-internal: "true"
```

## Subnet tagging for load balancers

You must have at least one tagged subnet in your VPC for NLB IP mode. If an AZ has multiple tagged subnets, the first one in lexicographical order by subnet ID is choosen.
For internet-facing load balancer, you must tag the public subnets as folows:

| Key | Value |
| --- | --- |
| `kubernetes.io/role/elb` | `1` |

For internal load balancer, you must tag the private subnets as follows:

| Key | Value |
| --- | --- |
|  `kubernetes.io/role/internal-elb`  |  `1`  |

## Protocols
Support is available for both TCP and UDP protocols. In case of TCP, NLB in IP mode does not pass the client source IP address to the pods. You can configure protocol v2 via annotation if you need the client source IP address.

## Security group
NLB does not currently support a managed security group. For ingress access, the controller will resolve the security group for the ENI corresponding tho the endpoint pod. If the ENI has a single security group, it gets used. In case of multiple security gropus, the controller expects to find only one security group tagged with the Kubernetes cluster id. Controller will update the ingress rules on the security groups as per the service spec.

## Annotations
Support is available for all of the NLB instance mode annotations. Annotation values must be strings,
* boolean: 'true'
* integer: '42'
* stringMap: k1=v1,k2=v2
* stringList: s1,s2,s3
* json: 'jsonContent'

|Name                                                                               | Type              | Default       |
|-----------------------------------------------------------------------------------|-------------------|---------------|
|service.beta.kubernetes.io/aws-load-balancer-type                                  | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-internal                              | boolean           | false         |
|service.beta.kubernetes.io/aws-load-balancer-proxy-protocol                        | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-access-log-enabled                    | boolean           | false         |
|service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name             | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix           | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled     | boolean           | false         |
|service.beta.kubernetes.io/aws-load-balancer-ssl-cert                              | stringList        |               |
|service.beta.kubernetes.io/aws-load-balancer-ssl-ports                             | stringList        |               |
|service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy                | string            | ELBSecurityPolicy-2016-08            |
|service.beta.kubernetes.io/aws-load-balancer-backend-protocol                      | string            |               |
|service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags              | stringMap         |               |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold         | integer           | 3             |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold       | integer           | 3             |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout                   | integer           | 10            |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval                  | integer           | 10            |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol                  | string            | TCP           |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-port                      | string            | traffic-port  |
|service.beta.kubernetes.io/aws-load-balancer-healthcheck-path                      | string            | "/" for HTTP(S) protocols |
|service.beta.kubernetes.io/aws-load-balancer-eip-allocations			    | stringList	|		|
