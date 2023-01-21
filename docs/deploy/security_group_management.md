In the context of the AWS LB controller, we can classify security groups into two categories:

1. Frontend SG
2. Backend SG

## Frontend SG

Frontend security groups control the clients that can access the load balancer. The frontend security groups can be configured via the `alb.ingress.kubernetes.io/security-groups` annotation on the ingress resource. If the annotation is not specified, controller will auto-create one security group per load balancer to allow the traffic from `inbound-cidrs` to `listen-ports`.

## Backend SG

Backend security group controls the traffic between the load balancer and the EC2 instances or the ENIs. This SG is attached to the load balancer and is used as traffic source in the ENI/Instance SG rules. The backend security group is shared between multiple load balancers.

The controller flag `--enable-backend-security-group` (default `true`) is used to enable/disable the shared backend SG. The flag `--backend-security-group` (default empty) is used to pass in the SG to use as a shared backend SG. If it is empty, the controller will auto-generate a SG with the following name and tags -

```
name: k8s-<cluster_name>-traffic-<hash of vpc, cluster name>
tags: 
    elbv2.k8s.aws/cluster: <cluster_name>
    elbv2.k8s.aws/type: backend
```

### Management of Backend SG rules

When the controller auto-creates the frontend SG for a load balancer, it automatically adds the security group rules to allow traffic from the load balancer to the EC2/Fargate instances.

In case security group is specified via annotation, the SG rules do not get added by default. The automatic management of instance/ENI security group can be controlled via the additional annotation `alb.ingress.kubernetes.io/manage-backend-security-group-rules` on the ingress resource. When this annotation is specified, SG rules are automatically managed if the value is true, and not managed if the value is false. This annotation gets ignored in case of auto-generated security groups.

#### Port range restriction for Backend SG rules

Starting with version v2.3.0, the default behavior for backend security group rules is to restrict them to specific port ranges. You can set the controller flag `--disable-restricted-sg-rules` to `true` to get the backend SG rules to allow ALL traffic.
