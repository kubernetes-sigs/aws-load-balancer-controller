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

The ALB Ingress Controller is configured by Annotations on the `Ingress` and `Service` resource objects.

```
alb.ingress.kubernetes.io/load-balancer-attributes
alb.ingress.kubernetes.io/backend-protocol
alb.ingress.kubernetes.io/certificate-arn
alb.ingress.kubernetes.io/healthcheck-interval-seconds
alb.ingress.kubernetes.io/healthcheck-path
alb.ingress.kubernetes.io/healthcheck-port
alb.ingress.kubernetes.io/healthcheck-protocol
alb.ingress.kubernetes.io/healthcheck-timeout-seconds
alb.ingress.kubernetes.io/healthy-threshold-count
alb.ingress.kubernetes.io/unhealthy-threshold-count
alb.ingress.kubernetes.io/listen-ports
alb.ingress.kubernetes.io/target-type
alb.ingress.kubernetes.io/scheme
alb.ingress.kubernetes.io/security-groups
alb.ingress.kubernetes.io/subnets
alb.ingress.kubernetes.io/success-codes
alb.ingress.kubernetes.io/tags
alb.ingress.kubernetes.io/target-group-attributes
alb.ingress.kubernetes.io/ignore-host-header
alb.ingress.kubernetes.io/ip-address-type
alb.ingress.kubernetes.io/ssl-policy
alb.ingress.kubernetes.io/actions.<ACTION NAME>
```

- **load-balancer-attributes**: Defines [Load Balancer Attributes](http://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_LoadBalancerAttribute.html) that should be applied to the ALB. This can be used to enable the S3 access logs feature of the ALB. Example: `alb.ingress.kubernetes.io/attributes: access_logs.s3.enabled=true,access_logs.s3.bucket=my-access-log-bucket`

- **backend-protocol**: Enables selection of protocol for ALB to use to connect to backend service. When omitted, `HTTP` is used.

- **certificate-arn**: Enables HTTPS and uses the certificate defined, based on arn, stored in your [AWS Certificate Manager](https://aws.amazon.com/certificate-manager).

- **healthcheck-interval-seconds**: The approximate amount of time, in seconds, between health checks of an individual target. The default is 15 seconds.

- **healthcheck-path**: The ping path that is the destination on the targets for health checks. The default is /.

- **healthcheck-port**: The port the load balancer uses when performing health checks on targets. The default is traffic-port, which indicates the port on which each target receives traffic from the load balancer.

- **healthcheck-protocol**: The protocol the load balancer uses when performing health checks on targets. The default is the HTTP protocol.

- **healthcheck-timeout-seconds**: The amount of time, in seconds, during which no response from a target means a failed health check. The default is 5 seconds.

- **healthcheck-healthy-threshold-count**: The number of consecutive health checks successes required before considering an unhealthy target healthy. The default is 2.

- **healthcheck-unhealthy-threshold-count**: The number of consecutive health check failures required before considering a target unhealthy. The default is 2.

- **listen-ports**: Defines the ports the ALB will expose. It defaults to `[{"HTTP": 80}]` unless a certificate ARN is defined, then it is `[{"HTTPS": 443}]`. Uses a format as follows '[{"HTTP":8080,"HTTPS": 443}]'.

- **target-type**: Defines if the EC2 instance ID or the pod IP are used in the managed Target Groups. Defaults to `instance`. Valid options are `instance` and `pod`. With `instance` the Target Group targets are `<ec2 instance id>:<node port>`, for `pod` the targets are `<pod ip>:<pod port>`. When using the pod IP, it will route from all availabilty zones. `pod` is to be used when the pod network is routable and can be reached by the ALB.

- **scheme**: Defines whether an ALB should be `internal` or `internet-facing`. See [Load balancer scheme](http://docs.aws.amazon.com/elasticloadbalancing/latest/userguide/how-elastic-load-balancing-works.html#load-balancer-scheme) in the AWS documentation for more details.

- **security-groups**: [Security groups](http://docs.aws.amazon.com/AmazonVPC/latest/UserGuide/VPC_SecurityGroups.html) that should be applied to the ALB instance. These can be referenced by security group IDs or the name tag associated with each security group. Example ID values are `sg-723a380a,sg-a6181ede,sg-a5181edd`. Example tag values are `appSG, webSG`. When the annotation is not present, the controller will create a security group with appropriate ports allowing access to `0.0.0.0/0` and attached to the ALB. It will also create a security group for instances that allows all TCP traffic when the source is the security group created for the ALB.

- **subnets**: The subnets where the ALB instance should be deployed. Must include 2 subnets, each in a different [availability zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html). These can be referenced by subnet IDs or the name tag associated with the subnet. Example values for subnet IDs are `subnet-a4f0098e,subnet-457ed533,subnet-95c904cd`. Example values for name tags are: `webSubnet,appSubnet`. If subnets are not specified the ALB controller will attempt to detect qualified subnets. This qualification is done by locating subnets that match the following criteria.

  - `kubernetes.io/cluster/$CLUSTER_NAME` where `$CLUSTER_NAME` is the same cluster name specified on the ingress controller. The value of this tag must be `shared` or `owned`.

  - `kubernetes.io/role/internal-elb` should be set for internal load balancers.
  - `kubernetes.io/role/elb` should be set for internet-facing load balancers.

  - After subnets matching the above 2 tags have been located, they are checked to ensure 2 or more are in unique AZs, otherwise the ALB will not be created. If 2 subnets share the same AZ, only 1 of the 2 is used.

- **success-codes**: Defines the HTTP status code that should be expected when doing health checks against the defined `healthcheck-path`. When omitted, `200` is used.

- **tags**: Defines [AWS Tags](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Tags.html) that should be applied to the ALB instance and Target groups.

- **target-group-attributes**: Defines [Target Group Attributes](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html#target-group-attributes) which can be assigned to the Target Groups. Currently these are applied equally to all target groups in the ingress.

- **ignore-host-header**: Creates routing rules without [Host Header Checks](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html#host-conditions).

- **ip-address-type**: The IP address type thats used to either route IPv4 traffic only or to route both IPv4 and IPv6 traffic. Can be either `dualstack` or `ipv4`. When omitted `ipv4` is used.

- **ssl-policy**: Defines the [Security Policy](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/create-https-listener.html#describe-ssl-policies) that should be assigned to the ALB, allowing you to control the protocol and ciphers.

- **alb.ingress.kubernetes.io/actions.\<ACTION NAME>**: Provides a method for configuring custom actions on a listener, such as for [Redirect Actions](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html#redirect-actions). The `<ACTION NAME>` in the annotation must match the `serviceName` in the ingress rules. The value of the annotation is the JSON spec of the action. See the [Action type](https://docs.aws.amazon.com/sdk-for-go/api/service/elbv2/#Action) for documentation on what should be in the JSON. An example for a fixed-response would be: `alb.ingress.kubernetes.io/actions.fixed-response-error: '{"Type": "fixed-response", "FixedResponseConfig": {"ContentType":"text/plain", "StatusCode":"503", "MessageBody":"503 error text"}}'` for a `serviceName` of `fixed-response-error`.

### Services

A subset of these annotations are supported on Services. This is used to customize the Target Group created for the Service. If a Service has no annotations, the Target Group options will default to the same options configured on the Ingress.

#### Optional Service Annotations

```
alb.ingress.kubernetes.io/backend-protocol
alb.ingress.kubernetes.io/healthcheck-interval-seconds
alb.ingress.kubernetes.io/healthcheck-path
alb.ingress.kubernetes.io/healthcheck-port
alb.ingress.kubernetes.io/healthcheck-protocol
alb.ingress.kubernetes.io/healthcheck-timeout-seconds
alb.ingress.kubernetes.io/healthy-threshold-count
alb.ingress.kubernetes.io/unhealthy-threshold-count
alb.ingress.kubernetes.io/target-type
alb.ingress.kubernetes.io/success-codes
alb.ingress.kubernetes.io/target-group-attributes
```
