# Using AWS Load Balancer Controller with Karpenter

[Karpenter](https://karpenter.sh/) is a Kubernetes node provisioner that automatically launches right-sized compute resources in response to unschedulable pods. When using Karpenter alongside the AWS Load Balancer Controller, there are a few important considerations.

## Security Group Discovery

The AWS Load Balancer Controller needs to identify the correct security groups for your nodes. When using Karpenter, nodes are provisioned dynamically and use security groups defined in the `EC2NodeClass` resource.

Ensure the security groups used by Karpenter-provisioned nodes have the `kubernetes.io/cluster/<cluster-name>: owned` tag. Without this tag, the controller may fail with "Multiple untagged security groups" when creating LoadBalancer-type Services.

```yaml
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: default
spec:
  securityGroupSelectorTerms:
    - tags:
        karpenter.sh/discovery: "${CLUSTER_NAME}"
  # Ensure the selected security groups are tagged with
  # kubernetes.io/cluster/<cluster-name>: owned
```

## Target Group Registration

When ALB Ingress or NLB Services target Karpenter-managed nodes:

- **Instance targets** are automatically registered/deregistered as Karpenter scales nodes up and down. The controller watches for node changes and updates target groups accordingly.
- **IP targets** (recommended) register pod IPs directly, which is independent of node lifecycle. This is the default mode and works seamlessly with Karpenter's dynamic provisioning.

To use IP targeting (default for ALBs):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    alb.ingress.kubernetes.io/target-type: ip
```

## Graceful Node Termination

Karpenter drains nodes before terminating them, which gives the Load Balancer Controller time to deregister targets. To ensure smooth draining:

1. Configure appropriate `terminationGracePeriodSeconds` on your pods
2. Use `alb.ingress.kubernetes.io/target-group-attributes: deregistration_delay.timeout_seconds=30` to control how long ALB waits for in-flight requests to complete
3. Karpenter respects PodDisruptionBudgets during consolidation and drift, so ensure your services have PDBs configured

## Topology Spread with ALB

When using `topologySpreadConstraints` with Karpenter, keep in mind that Karpenter provisions nodes across availability zones by default. This works well with ALB's cross-zone load balancing:

```yaml
spec:
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfied: DoNotSchedule
      labelSelector:
        matchLabels:
          app: my-app
```
