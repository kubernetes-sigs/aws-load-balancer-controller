---
title: Self Managed Load Balancers
---

## Motivation

The load balancer controller (LBC) generally creates and destroys [AWS Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/index.html) in response to Kubernetes resources. 

However, some cluster operators may prefer to manually manage AWS Load Balancers. This supports use cases like:
- Preventing acciential release of key IP addresses.
- Supporting load balancers where the Kubernetes cluster is one of multiple targets.
- Complying with organizational requirements controlling provisioning of load balancers for security or cost reasons. 

## Solution Overview

Use the TargetGroupBinding CRD to sync a Kubernetes service with the targets of a load balancer. 

First, a load balancer is manually created directly with AWS. A [listener](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html) and a [target group](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html) is added to the load balancer. 

Second, a TargetGroupBinding CRD is created in a cluster. The CRD includes references to a Kubernetes service and the [ARN](https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html) of the Load Balancer Target Group. The CRD configures the LBC to watch the service and automatically update the target group with the appropriate pod VPC IP addresses. 

## Prerequisites

- LBC Installed
- VPC ID of EKS Cluster
- Subnet IDs of Pods
- Port and Protocol of Target Service
    - `kubectl get service -A`

- AWS CLI
- Kubectl

## Configure Load Balancer

**Create Load Balancer: (optional)**

1. Use the [create\-load\-balancer](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-load-balancer.html) command to create an IPv4 load balancer, specifying a public subnet for each Availability Zone in which you launched instances. 

    [[NOTE: desc how to find your subnets]]

    `kubernetes.io/cluster/your-cluster-name`

    You can specify only one subnet per Availability Zone. 

    ```
    aws elbv2 create-load-balancer --name my-load-balancer --type network --subnets subnet-0e3f5cac72EXAMPLE
    ```

    **Important:** The output includes the ARN of the load balancer. This value is needed to configure the LBC. 

    Example:

    ```
    arn:aws:elasticloadbalancing:us-east-2:123456789012:loadbalancer/net/my-load-balancer/1234567890123456
    ```


1. Use the [create\-target\-group](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-target-group.html) command to create an IPv4 target group, specifying the same VPC of your EKS cluster. 

[[note: advise on instance vs IP target types?]]

   ```
   aws elbv2 create-target-group --name my-targets --protocol TCP --port 80 --vpc-id vpc-0598c7d356EXAMPLE
   ```

   The output includes the ARN of the target group, with this format:

   ```
   arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/my-targets/1234567890123456
   ```

1. Use the [create\-listener](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-listener.html) command to create a listener for your load balancer with a default rule that forwards requests to your target group. The listener port and protocol must match the Kubernetes service. However, TLS termination is permitted. [[double check it works in this configuration?]]

   ```
   aws elbv2 create-listener --load-balancer-arn loadbalancer-arn --protocol TCP --port 80  \
   --default-actions Type=forward,TargetGroupArn=targetgroup-arn
   ```

## Create TargetGroupBinding CRD

1. Create the [TargetGroupBinding CRD](/guide/targetgroupbinding/targetgroupbinding.md)

Insert the ARN of the Target Group, as created above.

Insert the name and port of the target Kubernetes service.

```yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: TargetGroupBinding
metadata:
  name: my-tgb
spec:
  serviceRef:
    name: awesome-service # route traffic to the awesome-service
    port: 80
  targetGroupARN: <arn-to-targetGroup>
```
2. Apply the CRD

`kubectl apply -f my-tgb.yaml`

## Verify

Wait approximately 30 seconds for the LBC to update the load balancer.

[View all target groups](https://console.aws.amazon.com/ec2/v2/home#TargetGroups:) in the AWS console.

Find the target group by the ARN noted above, and verify the EKS Cluster IP addresses have beened added.