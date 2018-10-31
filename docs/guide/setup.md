# Setup

This document describes how to setup the alb-ingress-controller using kubectl or helm. Additionally, it details how to setup [external-dns](https://github.com/kubernetes-incubator/external-dns) to work with the controller.

If you'd prefer an end-to-end walkthrough (example) of setup instead, see [the echoservice example](walkthrough.md).

## Prerequisites

This section details what must be setup in order for the controller to run.

### Kubelet

The [kubelet](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/) must be run with `--cloud-provider=aws`. This populates the EC2 instance ID in each node's spec.

### Role Permissions

Adequate roles and policies must be configured in AWS and available to the node(s) running the controller. How access is granted is up to you. Some will attach the needed rights to node's role in AWS. Others will use projects like [kube2iam](https://github.com/jtblin/kube2iam).

An example policy with the minimum rights can be found at [examples/iam-policy.json](../examples/iam-policy.json).

### Subnet Selection

The controller determines subnets to deploy each ALB to based on an annotation or auto-detection.

##### Via annotation

`alb.ingress.kubernetes.io/subnets` may be specified in each ingress resource with the subnet IDs or `Name` tags. This allows for flexibility in where ALBs land. This list of subnets must include 2 or more that exist in unique availability zones. See the [annotations documentation](../api/ingress.md#annotations) for more details.

##### Via tags on the subnets

When subnet annotations are not present, the controller will attempt to choose the best subnets for deploying the ALBs. It uses the following tag criteria to determine the subnets it should use.

- `kubernetes.io/cluster/$CLUSTER_NAME` equal to `shared` or `owned`. `$CLUSTER_NAME` must match the `CLUSTER_NAME` environment variable on the controller.

And one of the following:

- `kubernetes.io/role/internal-elb: ""` For internal load balancers
- `kubernetes.io/role/elb = ""` For internet-facing load balancers

### Security Group Selection

The controller determines if it should create and manage security groups or use existing ones in AWS based on the presence of an annotation. When `alb.ingress.kubernetes.io/security-groups` is present, the list of security groups is assigned to the ALB instance. When the annotation is not present, the controller will create a security group with appropriate ports allowing access to `0.0.0.0/0` and attached to the ALB. It will also create a security group for instances that allows all TCP traffic when the source is the security group created for the ALB.

## Helm Deployments

You must have the [Helm App Registry plugin](https://coreos.com/apps) installed for these instructions to work.

```
helm registry install quay.io/coreos/alb-ingress-controller-helm
```

## kubectl Deployments

1.  Configure the alb-ingress-controller manifest.

    A sample manifest can be found below.

    ```
    $ wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/master/docs/examples/alb-ingress-controller.yaml
    ```

    At minimum, edit the following variables.

    - `AWS_REGION`: region in AWS this cluster exists.

          	```yaml
          	- name: AWS_REGION
          	  value: us-west-1
          	```

    - `CLUSTER_NAME`: name of the cluster. If doing auto-detection of subnets (described in prerequisites above) `CLUSTER_NAME` must match the AWS tags associated with the subnets you wish ALBs to be provisioned.

          	```yaml
          	- name: CLUSTER_NAME
          	  value: devCluster
          	```

1.  Deploy the alb-ingress-controller manifest.

    ```
    $ kubectl apply -f alb-ingress-controller.yaml
    ```

1.  Verify the deployment was successful and the controller started.

    ```bash
    $ kubectl logs -n kube-system $(kubectl get po -n kube-system | egrep -o alb-ingress[a-zA-Z0-9-]+)
    ```

    Should display output similar to the following.

    ```
    -------------------------------------------------------------------------------
    AWS ALB Ingress controller
    Release:    UNKNOWN
    Build:      UNKNOWN
    Repository: UNKNOWN
    -------------------------------------------------------------------------------

    I0725 11:22:06.464996   16433 main.go:159] Creating API client for http://localhost:8001
    I0725 11:22:06.563336   16433 main.go:203] Running in Kubernetes cluster version v1.8+ (v1.8.9+coreos.1) - git (clean) commit cd373fe93e046b0a0bc7e4045af1bf4171cea395 - platform linux/amd64
    I0725 11:22:06.566255   16433 alb.go:80] ALB resource names will be prefixed with 2f92da62
    I0725 11:22:06.645910   16433 alb.go:163] Starting AWS ALB Ingress controller
    ```

## external-dns Deployment

[external-dns](https://github.com/kubernetes-incubator/external-dns) provisions DNS records based on the host information. This project will setup and manage records in Route 53 that point to controller deployed ALBs.

1.  Ensure your instance has the correct IAM permission required for external-dns. See https://github.com/kubernetes-incubator/external-dns/blob/master/docs/tutorials/aws.md#iam-permissions.

1.  Download external-dns to manage Route 53.

    ```bash
    $ wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/master/docs/examples/external-dns.yaml
    ```

1.  Edit the `--domain-filter` flag to include your hosted zone(s)

    The following example is for a hosted zone test-dns.com

    ```yaml
    args:
    - --source=service
    - --source=ingress
    - --domain-filter=test-dns.com # will make ExternalDNS see only the hosted zones matching provided domain, omit to process all available hosted zones
    - --provider=aws
    - --policy=upsert-only # would prevent ExternalDNS from deleting any records, omit to enable full synchronization
    ```

1.  Deploy external-dns

    ```
    $ kubectl apply -f external-dns.yaml
    ```

1.  Verify it deployed successfully.

    ```
    $ kubectl logs -f -n kube-system $(kubectl get po -n kube-system | egrep -o 'external-dns[A-Za-z0-9-]+')

    time="2017-09-19T02:51:54Z" level=info msg="config: &{Master: KubeConfig: Sources:[service ingress] Namespace: FQDNTemplate: Compatibility: Provider:aws GoogleProject: DomainFilter:[] AzureConfigFile:/etc/kuberne tes/azure.json AzureResourceGroup: Policy:upsert-only Registry:txt TXTOwnerID:my-identifier TXTPrefix: Interval:1m0s Once:false DryRun:false LogFormat:text MetricsAddress::7979 Debug:false}"
    time="2017-09-19T02:51:54Z" level=info msg="Connected to cluster at https://10.3.0.1:443"
    ```
