# Subnet Auto-Discovery
The AWS Load Balancer Controller (LBC) automatically discovers subnets for creating AWS Network Load Balancers (NLB) and AWS Application Load Balancers (ALB). This auto-discovery process follows three main steps:

1. **Candidate Subnet Determination**: The controller identifies potential candidate subnets
2. **Subnet Filtering**: The controller filters these candidates based on eligibility criteria
3. **Final Selection**: The controller selects one subnet per availability zone

## Candidate Subnet Determination
The controller determines candidate subnets using the following process:

1. **If tag filters are specified**: Only subnets matching these filters become candidates
   
    !!!tip
        You can only specify subnet tag filters for Ingress via IngressClassParams

2. **If no tag filters are specified**:
    * If subnets with matching [role tag](#subnet-role-tag) exists: Only these become candidates
    * [**For LBC version >= 2.12.1**] If no subnets have role tags: Candidates are subnets whose [reachability](#subnet-reachability) (public/private) matches the LoadBalancer's schema


### Subnet Role Tag
Subnets can be tagged appropriately for auto-discovery selection:

* **For internet-facing load balancers**, the controller looks for public subnets with following tags:

    | Key                                     | Value                 |
    | --------------------------------------- | --------------------- |
    | `kubernetes.io/role/elb`                | `1`  or ``            |

* **For internal load balancers**, the controller looks for private subnets with following tags:

    | Key                                     | Value                 |
    | --------------------------------------- | --------------------- |
    |  `kubernetes.io/role/internal-elb`      |  `1`  or ``           |

### Subnet reachability
The controller automatically discovers all subnets in your VPC and determines whether each is a public or private subnet based on its associated route table configuration.
A subnet is classified as public if its route table contains a route to an Internet Gateway.

!!!tip
    You can disable this behavior via SubnetDiscoveryByReachability feature flag.

## Subnet Filtering

1. **Cluster Tag Check**: The controller checks for cluster tags on subnets. Subnets with ineligible cluster tags will be filtered out.

    | Key                                     | Value                 |
    | --------------------------------------- | --------------------- |
    |  `kubernetes.io/cluster/${cluster-name}`      |  `owned` or `shared`          |

    * If such cluster tag exists but no `<clusterName>` matches the current cluster, those subnets will be filtered out.
    * [**For LBC version < 2.1.1**] subnets without a cluster tag matching cluster name will be filtered out.

    !!! tip
        You can disable this behavior via the `SubnetsClusterTagCheck` feature flag. When disabled, no cluster tag check will be performed against subnets.

2. **IP Address Availability**: Subnets with insufficient available IP addresses(**<8**) are filtered out.

## Final Selection

The controller selects one subnet per availability zone. When multiple subnets exist per Availability Zone, the following priority order applies:

1. Subnets with cluster tag for the current cluster (`kubernetes.io/cluster/<clusterName>`) are prioritized
2. Subnets with lower lexicographical order of subnet ID are prioritized

## Minimum Subnet Requirements

* **ALBs**: Require at least two subnets across different Availability Zones by default

    !!! tip
        For customers allowlisted by the AWS Elastic Load Balancing team, you can enable the [ALBSingleSubnet feature gate](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/deploy/configurations/#feature-gates). This allows provisioning an ALB with just one subnet instead of the standard requirement of two subnets.