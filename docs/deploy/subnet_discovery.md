# Subnet Auto Discovery
AWS Load Balancer controller auto discovers network subnets for ALB or NLB by default. ALB requires at least two subnets across Availability Zones, NLB requires one subnet.
The subnets must be tagged appropriately for the auto discovery to work. The controller chooses one subnet from each Availability Zone. During auto-discovery, the controller
considers subnets with at lease 8 available ip addresses. In case of multiple qualified tagged subnets in an Availability Zone, the controller chooses the first one in lexicographical 
order by the Subnet IDs. For more information about the subnets for the AWS load balancer, see [Application Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html) 
and [Network Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/network-load-balancers.html).
If you use `eksctl` or an Amazon EKS AWS CloudFormation template to create your VPC after March 26, 2020, then the subnets are tagged appropriately when they're created. For 
more information about the Amazon EKS AWS CloudFormation VPC templates, see [Creating a VPC for your Amazon EKS cluster](https://docs.aws.amazon.com/eks/latest/userguide/create-public-private-vpc.html).

## Public subnets
Public subnets are used for internet-facing load balancers. These subnets must have the following tags:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/role/elb`                | `1`  or ``            |

## Private subnets
Private subnets are used for internal load balancers. These subnets must have the following tags:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
|  `kubernetes.io/role/internal-elb`      |  `1`  or ``           |


## Common tag
In version v2.1.1 and older, both the public and private subnets must be tagged with the cluster name as follows:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/cluster/${cluster-name}` | `owned` or `shared`   |

 `${cluster-name}` is the name of the kubernetes cluster
 
The cluster tag is not required in versions from v2.1.2 to v2.4.1, unless a cluster tag for another cluster is present.

Starting from v2.4.2 release, you can disable the cluster tag check completely by specifying the feature gate `SubnetsClusterTagCheck=false`
