The AWS Load Balancer Controller classifies security groups into two categories: frontend and backend.

## Frontend Security Groups

Frontend security groups control which clients can access the load balancers. The frontend security groups can be configured with the `alb.ingress.kubernetes.io/security-groups` annotation on the Ingress resources. If the annotation is not specified, the LBC will create one security group per load balancer, allowing traffic from `inbound-cidrs` to `listen-ports`.

## Backend Security Groups

A single backend security group controls the traffic between load balancers and their target EC2 instances or ENIs. This security group is attached to the load balancers and is used as the traffic source in the ENI/Instance security group rules. The backend security group is shared between multiple load balancers.

The controller flag `--enable-backend-security-group` (default `true`) is used to enable/disable the shared backend security group. The flag `--backend-security-group` (default empty) is used to pass in the security group to use as a shared backend security group. If it is empty, the LBC will auto-generate a security group with the following name and tags -

```
name: k8s-traffic-<cluster_name>-<hash_of_cluster_name>
tags: 
    elbv2.k8s.aws/cluster: <cluster_name>
    elbv2.k8s.aws/resource: backend-sg
```

### Management of Backend Security Group Rules

When the LBC auto-creates the backend security group for a load balancer, it automatically adds the security group rules to allow traffic from the load balancer to the EC2/Fargate instances.

In case security group is specified via annotation, the security group rules do not get added by default. The automatic management of instance/ENI security group can be controlled via the additional annotation `alb.ingress.kubernetes.io/manage-backend-security-group-rules` on the ingress resource. When this annotation is specified, security group rules are automatically managed if the value is true, and not managed if the value is false. This annotation gets ignored in case of auto-generated security groups.

### Port Range Restrictions for Backend Security Group Rules

Starting with version v2.3.0, the default behavior for backend security group rules is to restrict them to specific port ranges. You can set the controller flag `--disable-restricted-sg-rules` to `true` to get the backend security group rules to allow ALL traffic.
