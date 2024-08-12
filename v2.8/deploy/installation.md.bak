# AWS Load Balancer Controller installation

The AWS Load Balancer controller (LBC) provisions AWS Network Load Balancer (NLB) and Application Load Balancer (ALB) resources. The LBC watches for new `service` or `ingress` Kubernetes resources and configures AWS resources.

The LBC is supported by AWS. Some clusters may be using the legacy "in-tree" functionality to provision AWS load balancers. The AWS Load Balancer Controller should be installed instead.

!!!question "Existing AWS ALB Ingress Controller users"
    The AWS ALB Ingress controller must be uninstalled before installing the AWS Load Balancer Controller.
    Please follow our [migration guide](upgrade/migrate_v1_v2.md) to do a migration.
    
!!!warning "When using AWS Load Balancer Controller v2.5+"
    The AWS LBC provides a mutating webhook for service resources to set the `spec.loadBalancerClass` field for service of type LoadBalancer on create. 
    This makes the AWS LBC the **default controller for service** of type LoadBalancer. You can disable this feature and revert to set Cloud Controller Manager (in-tree controller) as the default by setting the helm chart value **enableServiceMutatorWebhook to false** with `--set enableServiceMutatorWebhook=false` . 
    You will no longer be able to provision new Classic Load Balancer (CLB) from your kubernetes service unless you disable this feature. Existing CLB will continue to work fine.

## Supported Kubernetes versions
* AWS Load Balancer Controller v2.0.0~v2.1.3 requires Kubernetes 1.15+
* AWS Load Balancer Controller v2.2.0~v2.3.1 requires Kubernetes 1.16-1.21
* AWS Load Balancer Controller v2.4.0+ requires Kubernetes 1.19+
* AWS Load Balancer Controller v2.5.0+ requires Kubernetes 1.22+

## Deployment considerations

### Additional requirements for non-EKS clusters:

* Ensure subnets are tagged appropriately for auto-discovery to work
* For IP targets, pods must have IPs from the VPC subnets. You can configure the [`amazon-vpc-cni-k8s`](https://github.com/aws/amazon-vpc-cni-k8s#readme) plugin for this purpose.

### Additional requirements for isolated cluster:
Isolated clusters are clusters without internet access, and instead reply on VPC endpoints for all required connects.
When installing the AWS LBC in isolated clusters, you need to disable shield, waf and wafv2 via controller flags `--enable-shield=false, --enable-waf=false, --enable-wafv2=false`
### Using the Amazon EC2 instance metadata server version 2 (IMDSv2)
We recommend blocking the access to instance metadata by requiring the instance to use [IMDSv2](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-instance-metadata-service.html) only. For more information, please refer to the AWS guidance [here](https://aws.github.io/aws-eks-best-practices/security/docs/iam/#restrict-access-to-the-instance-profile-assigned-to-the-worker-node). If you are using the IMDSv2, set the hop limit to 2 or higher in order to allow the LBC to perform the metadata introspection. 

You can set the IMDSv2 as follows:
```
aws ec2 modify-instance-metadata-options --http-put-response-hop-limit 2 --http-tokens required --region <region> --instance-id <instance-id>
```

Instead of depending on IMDSv2, you can specify the AWS Region and the VPC via the controller flags `--aws-region` and `--aws-vpc-id`.

## Configure IAM

The controller runs on the worker nodes, so it needs access to the AWS ALB/NLB APIs with IAM permissions.

The IAM permissions can either be setup using [IAM roles for service accounts (IRSA)](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) or can be attached directly to the worker node IAM roles. The best practice is using IRSA if you're using Amazon EKS. If you're using kOps or self-hosted Kubernetes, you must manually attach polices to node instances.

### Option A: Recommended, IAM roles for service accounts (IRSA)

The reference IAM policies contain the following permissive configuration:
```
{
    "Effect": "Allow",
    "Action": [
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:RevokeSecurityGroupIngress"
    ],
    "Resource": "*"
},
```

We recommend further scoping down this configuration based on the VPC ID or cluster name resource tag.

Example condition for VPC ID:
```
    "Condition": {
        "ArnEquals": {
            "ec2:Vpc": "arn:aws:ec2:<REGION>:<ACCOUNT-ID>:vpc/<VPC-ID>"
        }
    }
```

Example condition for cluster name resource tag:
```
    "Condition": {
        "Null": {
            "aws:ResourceTag/kubernetes.io/cluster/<CLUSTER-NAME>": "false"
        }
    }
```

1. Create an IAM OIDC provider. You can skip this step if you already have one for your cluster.
    ```
    eksctl utils associate-iam-oidc-provider \
        --region <region-code> \
        --cluster <your-cluster-name> \
        --approve
    ```

2. Download an IAM policy for the LBC using one of the following commands:<p>
    If your cluster is in a US Gov Cloud region:
    ```
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.8.2/docs/install/iam_policy_us-gov.json
    ```
    If your cluster is in a China region:
    ```
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.8.2/docs/install/iam_policy_cn.json
    ```
    If your cluster is in any other region:
    ```
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.8.2/docs/install/iam_policy.json
    ```

3. Create an IAM policy named `AWSLoadBalancerControllerIAMPolicy`. If you downloaded a different policy, replace `iam-policy` with the name of the policy that you downloaded.
    ```
    aws iam create-policy \
        --policy-name AWSLoadBalancerControllerIAMPolicy \
        --policy-document file://iam-policy.json
    ```
    Take note of the policy ARN that's returned.

4. Create an IAM role and Kubernetes `ServiceAccount` for the LBC. Use the ARN from the previous step.
    ```
    eksctl create iamserviceaccount \
    --cluster=<cluster-name> \
    --namespace=kube-system \
    --name=aws-load-balancer-controller \
    --attach-policy-arn=arn:aws:iam::<AWS_ACCOUNT_ID>:policy/AWSLoadBalancerControllerIAMPolicy \
    --override-existing-serviceaccounts \
    --region <region-code> \
    --approve
    ```

### Option B: Attach IAM policies to nodes
If you're not setting up IAM roles for service accounts, apply the IAM policies from the following URL at a minimum. Please be aware of the possibility that the controller permissions may be assumed by other users in a pod after retrieving the node role credentials, so the best practice would be using IRSA instead of attaching IAM policy directly.
```
curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.8.2/docs/install/iam_policy.json
```

The following IAM permissions subset is for those using `TargetGroupBinding` only and don't plan to use the LBC to manage security group rules:

```
{
    "Statement": [
        {
            "Action": [
                "ec2:DescribeVpcs",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeInstances",
                "elasticloadbalancing:DescribeTargetGroups",
                "elasticloadbalancing:DescribeTargetHealth",
                "elasticloadbalancing:ModifyTargetGroup",
                "elasticloadbalancing:ModifyTargetGroupAttributes",
                "elasticloadbalancing:RegisterTargets",
                "elasticloadbalancing:DeregisterTargets"
            ],
            "Effect": "Allow",
            "Resource": "*"
        }
    ],
    "Version": "2012-10-17"
}
```

## Network configuration

Review the [worker nodes security group](https://docs.aws.amazon.com/eks/latest/userguide/sec-group-reqs.html) docs. Your node security group must permit incoming traffic on TCP port 9443 from the Kubernetes control plane. This is needed for webhook access.

If you use [eksctl](https://eksctl.io/usage/vpc-networking/), this is the default configuration.

If you use custom networking, please refer to the [EKS Best Practices Guides](https://aws.github.io/aws-eks-best-practices/networking/custom-networking/#use-custom-networking-when) for network configuration.
## Add controller to cluster

We recommend using the Helm chart to install the controller. The chart supports Fargate and facilitates updating the controller.

=== "Helm"

    If you want to run the controller on Fargate, use the Helm chart, since it doesn't depend on the `cert-manager`.

    ### Detailed instructions
    Follow the instructions in the [aws-load-balancer-controller](https://github.com/aws/eks-charts/tree/master/stable/aws-load-balancer-controller) Helm chart.

    ### Summary

    1. Add the EKS chart repo to Helm
    ```
    helm repo add eks https://aws.github.io/eks-charts
    ```
    2. If upgrading the chart via `helm upgrade`, install the `TargetGroupBinding` CRDs.
    ```
    wget https://raw.githubusercontent.com/aws/eks-charts/master/stable/aws-load-balancer-controller/crds/crds.yaml
    kubectl apply -f crds.yaml
    ```

        !!!tip
            The `helm install` command automatically applies the CRDs, but `helm upgrade` doesn't.


    Helm install command for clusters with IRSA:
    ```
    helm install aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=<cluster-name> --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller
    ```

    Helm install command for clusters not using IRSA:
    ```
    helm install aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=<cluster-name>
    ```



=== "YAML manifests"

    ### Install `cert-manager`

    ```
    kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.12.3/cert-manager.yaml
    ```

    ### Apply YAML
    1. Download the spec for the LBC.
    ```
    wget https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/download/v2.8.2/v2_8_2_full.yaml
    ```
    2. Edit the saved yaml file, go to the Deployment spec, and set the controller `--cluster-name` arg value to your EKS cluster name
    ```
    apiVersion: apps/v1
    kind: Deployment
    . . .
    name: aws-load-balancer-controller
    namespace: kube-system
    spec:
        . . .
        template:
            spec:
                containers:
                    - args:
                        - --cluster-name=<your-cluster-name>
    ```
    3. If you use IAM roles for service accounts, we recommend that you delete the `ServiceAccount` from the yaml spec. If you delete the installation section from the yaml spec, deleting the `ServiceAccount` preserves the `eksctl` created `iamserviceaccount`.
    ```
    apiVersion: v1
    kind: ServiceAccount
    ```
    4. Apply the yaml file
    ```
    kubectl apply -f v2_8_2_full.yaml
    ```
    5. Optionally download the default ingressclass and ingressclass params
    ```
    wget https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/download/v2.8.2/v2_8_2_ingclass.yaml
    ```
    6. Apply the ingressclass and params
    ```
    kubectl apply -f v2_8_2_ingclass.yaml
    ```

## Create Update Strategy

The controller doesn't receive security updates automatically. You need to manually upgrade to a newer version when it becomes available.

You can upgrade using [`helm upgrade`](https://helm.sh/docs/helm/helm_upgrade/) or another strategy to manage the controller deployment.
