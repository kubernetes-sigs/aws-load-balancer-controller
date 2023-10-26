---
title: Restrict Access with Frontend Security Groups
---

Frontend security groups limit client/internet traffic with a load balancer. This improves security by preventing unauthorized access to cluster services, and blocking unexpected outbound connections. Both [AWS Network Load Balancers (NLBs) and Application Load Balancers (ALBs)](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html) support frontend security groups. Learn more about how the Load Balancer Controller uses [Frontend and Backend Security Groups](../../../deploy/security_groups.md). 

## Solution Overview

Load balancers expose cluster workloads to a wider network. Creating a frontend security group limits access to these workloads (service or ingress resources). More specifically, a security group acts as a virtual firewall to control incoming and outgoing traffic. Inbound rules control the incoming traffic to your load balancer, and outbound rules control the outgoing traffic from your load balancer.

Security groups are particularly suited for defining what access other AWS resources (services, EC2 instances) have to your cluster. For example, if you have an existing security group including EC2 instances, you can permit only that security group to access a service.

In this example, you will restrict access to a cluster service. You will create a new security group for the frontend of a load balancer, and add an inbound rule permitting traffic. The rule may limit traffic to a specific port, CIDR, or existing security group.

## Prerequisites

- [Kubernetes Cluster Version 1.22+](https://docs.aws.amazon.com/cli/latest/reference/eks/describe-cluster.html)
- [AWS Load Balancer Controller v2.6.0+](../../../deploy/installation/)
- [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)

## Configure

### 1. Find the VPC ID of your cluster

```sh
$ aws eks describe-cluster --name <cluster-name> --query "cluster.resourcesVpcConfig.vpcId" --output text

vpc-0101XXXXa356
```

Ensure you have the right cluster name, AWS region, and the AWS CLI is configured.

### 2. Create a security group using the VPC ID

```sh
$ aws ec2 create-security-group --group-name <sg-name> --description <description> --vpc-id <vpc-id>

{
    "GroupId": "sg-0406XXXX645c"
}
```

Note the security group ID. This will be the frontend security group for the load balancer.

### 3. Create your ingress rules

Load balancers generally serve as an entrypoint for clients to access your cluster. This makes ingress rules especially important.

For example, this rule permits all traffic on port 443:

```sh
aws ec2 authorize-security-group-ingress --group-id <sg-id> --protocol all --port 443 --cidr 0.0.0.0/0
```

Learn more about how to [create an ingress rule with the AWS CLI.](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ec2/authorize-security-group-ingress.html)

### 4. Determine your egress rules (optional)

By default, all outbound traffic is allowed. Further, security groups are stateful, and responses to an allowed connection will also be permitted.

Learn how to [create an egress rule with the AWS CLI.](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ec2/authorize-security-group-egress.html)

### 5. Add the security group annotation to your Ingress or Service

For [Ingress resources](../../../guide/ingress/annotations.md), add the following annotation:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: frontend
  annotations:
    alb.ingress.kubernetes.io/security-groups: <sg-id>
```

For [Service resources](../../../guide/service/annotations.md#annotations), add the following annotation:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: frontend
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-security-groups: <sg-id>
spec:
    type: LoadBalancer
    loadBalancerClass: service.k8s.aws/nlb
```

For Ingress resources, the associated Application Load Balancer will be updated. For Service resources, the associated Network Load Balancer will be updated.

### 6. List your load balancers and verify the security groups are attached

```sh
$ aws elbv2 describe-load-balancers

{
    "LoadBalancers": [
        {
            "LoadBalancerArn": "arn:aws:elasticloadbalancing:us-east-1:1853XXXX5115:loadbalancer/net/k8s-default-frontend-ae3743b818/3ad6d16fb75ff688",
            <...>
            "SecurityGroups": [
                "sg-0406XXXX645c",
                "sg-0873XXXX2bef"
            ],
            "IpAddressType": "ipv4"
        }
    ]
}
```

If you don't see the security groups, verify:

- The Load Balancer Controller is properly installed.
- The controller has proper IAM permissions to modify load balancers. Look at the logs of the controller pods for IAM errors.

### 7. Clean up (Optional)

Removing the annotations from Service/Ingress resources will revert to the default frontend ecurity groups.

Load balancers may be costly. Delete Ingress and Service resources to deprovision the load balancers. If the load balancers are deleted from the console, they may be recreated by the controller.
