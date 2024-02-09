# Subnet auto-discovery
By default, the AWS Load Balancer Controller (LBC) auto-discovers network subnets that it can create AWS Network Load Balancers (NLB) and AWS Application Load Balancers (ALB) in. ALBs require at least two subnets across Availability Zones by default, 
set [Feature Gate ALBSingleSubnet](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/deploy/configurations/#feature-gates) to "true" allows using only 1 subnet for provisioning ALB. NLBs require one subnet.
The subnets must be tagged appropriately for auto-discovery to work. The controller chooses one subnet from each Availability Zone. During auto-discovery, the controller
considers subnets with at least eight available IP addresses. In the case of multiple qualified tagged subnets in an Availability Zone, the controller chooses the first one in lexicographical 
order by the subnet IDs.
For more information about the subnets for the LBC, see [Application Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html) 
and [Network Load Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/network-load-balancers.html).
If you used `eksctl` or an Amazon EKS AWS CloudFormation template to create your VPC after March 26, 2020, then the subnets are tagged appropriately when they're created. For 
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
In version v2.1.1 and older of the LBC, both the public and private subnets must be tagged with the cluster name as follows:

| Key                                     | Value                 |
| --------------------------------------- | --------------------- |
| `kubernetes.io/cluster/${cluster-name}` | `owned` or `shared`   |

 `${cluster-name}` is the name of the Kubernetes cluster.
 
The cluster tag is not required in versions v2.1.2 to v2.4.1, unless a cluster tag for another cluster is present.

With versions v2.4.2 and later, you can disable the cluster tag check completely by specifying the feature gate `SubnetsClusterTagCheck=false`
