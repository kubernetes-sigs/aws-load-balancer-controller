# Ingress Resources

This document covers how ingress resources work in relation to The ALB Ingress Controller.

## Ingress Behavior

Periodically, ingress update events are seen by the controller. The controller retains a list of all ingress resources it knows about, along with the current state of AWS components that satisfy them. When an update event is fired, the controller re-scans the list of ingress resources known to the cluster and determines, by comparing the list to its previously stored one, the ingresses requiring deletion, creation or modification.

An example ingress, from `example/2048/2048-ingress.yaml` is as follows.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: "nginx-ingress"
  namespace: "2048-game"
  annotations:
    kubernetes.io/ingress.class: alb
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

The host field specifies the eventual Route 53-managed domain that will route to this service. The service, service-2048, must be of type NodePort (see [../examples/echoservice/echoserver-service.yaml](../examples/echoservice/echoserver-service.yaml)) in order for the provisioned ALB to route to it. If no NodePort exists, the controller will not attempt to provision resources in AWS. For details on purpose of annotations seen above, see [Annotations](#annotations).

## Annotations

The ALB Ingress Controller is configured by Annotations on the `Ingress` resource object. Some are required and some are optional. All annotations use the namespace `alb.ingress.kubernetes.io/`.

### Required Annotations

```
alb.ingress.kubernetes.io/scheme
```

Required annotations are:

- **scheme**: Defines whether an ALB should be `internal` or `internet-facing`. See [Load balancer scheme](http://docs.aws.amazon.com/elasticloadbalancing/latest/userguide/how-elastic-load-balancing-works.html#load-balancer-scheme) in the AWS documentation for more details.

### Optional Annotations

```
alb.ingress.kubernetes.io/backend-protocol
alb.ingress.kubernetes.io/certificate-arn
alb.ingress.kubernetes.io/connection-idle-timeout
alb.ingress.kubernetes.io/healthcheck-interval-seconds
alb.ingress.kubernetes.io/healthcheck-path
alb.ingress.kubernetes.io/healthcheck-port
alb.ingress.kubernetes.io/healthcheck-protocol
alb.ingress.kubernetes.io/healthcheck-timeout-seconds
alb.ingress.kubernetes.io/healthy-threshold-count
alb.ingress.kubernetes.io/unhealthy-threshold-count
alb.ingress.kubernetes.io/listen-ports
alb.ingress.kubernetes.io/security-groups
alb.ingress.kubernetes.io/subnets
alb.ingress.kubernetes.io/successCodes
alb.ingress.kubernetes.io/tags
alb.ingress.kubernetes.io/target-group-attributes
alb.ingress.kubernetes.io/ignore-host-header
alb.ingress.kubernetes.io/ip-address-type
```

Optional annotations are:

- **backend-protocol**: Enables selection of protocol for ALB to use to connect to backend service. When omitted, `HTTP` is used.

- **certificate-arn**: Enables HTTPS and uses the certificate defined, based on arn, stored in your [AWS Certificate Manager](https://aws.amazon.com/certificate-manager).

- **connection-idle-timeout**: Sets the connection idle timeout setting for the ALB. This is a global setting for the ALB.

- **healthcheck-interval-seconds**: The approximate amount of time, in seconds, between health checks of an individual target. The default is 15 seconds.

- **healthcheck-path**: The ping path that is the destination on the targets for health checks. The default is /.

- **healthcheck-port**: The port the load balancer uses when performing health checks on targets. The default is traffic-port, which indicates the port on which each target receives traffic from the load balancer.

- **healthcheck-protocol**: The protocol the load balancer uses when performing health checks on targets. The default is the HTTP protocol.

- **healthcheck-timeout-seconds**: The amount of time, in seconds, during which no response from a target means a failed health check. The default is 5 seconds.

- **healthcheck-healthy-threshold-count**: The number of consecutive health checks successes required before considering an unhealthy target healthy. The default is 2.

- **healthcheck-unhealthy-threshold-count**: The number of consecutive health check failures required before considering a target unhealthy. The default is 2.

- **listen-ports**: Defines the ports the ALB will expose. When omitted, `80` is used for HTTP and `443` is used for HTTPS. Uses a format as follows '[{"HTTP":8080,"HTTPS": 443}]'.

- **security-groups**: [Security groups](http://docs.aws.amazon.com/AmazonVPC/latest/UserGuide/VPC_SecurityGroups.html) that should be applied to the ALB instance. These can be referenced by security group IDs or the name tag associated with each security group. Example ID values are `sg-723a380a,sg-a6181ede,sg-a5181edd`. Example tag values are `appSG, webSG`. When the annotation is not present, the controller will create a security group with appropriate ports allowing access to `0.0.0.0/0` and attached to the ALB. It will also create a security group for instances that allows all TCP traffic when the source is the security group created for the ALB.

- **subnets**: The subnets where the ALB instance should be deployed. Must include 2 subnets, each in a different [availability zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html). These can be referenced by subnet IDs or the name tag associated with the subnet.  Example values for subnet IDs are `subnet-a4f0098e,subnet-457ed533,subnet-95c904cd`. Example values for name tags are: `webSubnet,appSubnet`. If subnets are not specified the ALB controller will attempt to detect qualified subnets. This qualification is done by locating subnets that match the following criteria.

  - kubernetes.io/cluster/$CLUSTER_NAME where $CLUSTER_NAME is the same cluster name specified on the ingress controller. The value of this tag must be 'shared'.

  - kubernetes.io/role/alb-ingress the value of this tag should be empty.

  - After subnets matching the above 2 tags have been located, they are checked to ensure 2 or more are in unique AZs, otherwise the ALB will not be created. If 2 subnets share the same AZ, only 1 of the 2 is used.

- **successCodes**: Defines the HTTP status code that should be expected when doing health checks against the defined `healthcheck-path`. When omitted, `200` is used.

- **tags**: Defines [AWS Tags](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Tags.html) that should be applied to the ALB instance and Target groups.

- **target-group-attributes**: Defines [Target Group Attributes](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html#target-group-attributes) which can be assigned to the Target Groups. Currently these are applied equally to all target groups in the ingress.

- **ignore-host-header**: Creates routing rules without [Host Header Checks](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html#host-conditions).

- **ip-address-type**: The IP address type thats used to either route IPv4 traffic only or to route both IPv4 and IPv6 traffic. Can be either `dualstack` or `ipv4`. When omitted `ipv4` is used.
