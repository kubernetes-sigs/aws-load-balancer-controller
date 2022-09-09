# Load Balancer Controller Installation

The Load Balancer controller (LBC) provisions AWS Network Load Balancer (NLB) and Application Load Balancer (ALB) resources. The LBC watches for new service or ingress kubernetes resources, and configures AWS resources.

The LBC is supported by AWS. Some clusters may using legeacy "in-tree" functionality to provision AWS load balancers. The AWS Load Balancer Controller should be installed instead. 

!!!question "Existing AWS ALB Ingress Controller users"
    AWS ALB Ingress controller must be uninstalled before installing AWS Load Balancer controller.
    Please follow our [migration guide](upgrade/migrate_v1_v2.md) to do migration.

## Supported Kubernetes Versions 
* AWS Load Balancer Controller v2.0.0~v2.1.3 requires Kubernetes 1.15+
* AWS Load Balancer Controller v2.2.0~v2.3.1 requires Kubernetes 1.16-1.21
* AWS Load Balancer Controller v2.4.0+ requires Kubernetes 1.19+

## Deployment Considerations

### Additional Requirements for non-EKS clusters:

* Ensure subnets are tagged appropriately for auto-discovery to work
* For IP targets, pods must have IPs from the VPC subnets. You can configure `amazon-vpc-cni-k8s` plugin for this purpose.

### Using metadata server version 2 (IMDSv2)
If you are using the [IMDSv2](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-instance-metadata-service.html) you must set the hop limit to 2 or higher in order to allow the AWS Load Balancer Controller to perform the metadata introspection.

You can set the IMDSv2 hop limit as follows:
```
aws ec2 modify-instance-metadata-options --http-put-response-hop-limit 2 --region <region> --instance-id <instance-id>
```

Instead of depending on IMDSv2, you alternatively may specify the AWS region and the VPC via the controller flags `--aws-region` and `--aws-vpc-id`.

## Configure IAM

The controller runs on the worker nodes, so it needs access to the AWS ALB/NLB APIs via IAM permissions.

The IAM permissions can either be setup via [IAM roles for ServiceAccount (IRSA)](https://docs.aws.amazon.com/emr/latest/EMR-on-EKS-DevelopmentGuide/setting-up-enable-IAM.html) or can be attached directly to the worker node IAM roles. If you are using kops or vanilla k8s, polices must be manually attached to node instances.

### Option A: IAM Roles for Service Accounts (IRSA)

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

We recommend to further scope down this configuration based on the VPC ID or cluster name resource tag. 

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

1. Create IAM OIDC provider
    ```
    eksctl utils associate-iam-oidc-provider \
        --region <region-code> \
        --cluster <your-cluster-name> \
        --approve
    ```

1. Download IAM policy for the AWS Load Balancer Controller
    ```
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.4.3/docs/install/iam_policy.json
    ```

1. Create an IAM policy called AWSLoadBalancerControllerIAMPolicy
    ```
    aws iam create-policy \
        --policy-name AWSLoadBalancerControllerIAMPolicy \
        --policy-document file://iam-policy.json
    ```
    Take note of the policy ARN that is returned

1. Create a IAM role and ServiceAccount for the AWS Load Balancer controller, use the ARN from the step above
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
    
### Option B: Attach IAM Policies to Nodes
If not setting up IAM for ServiceAccount, apply the IAM policies from the following URL at minimum.
```
curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.4.3/docs/install/iam_policy.json
```

*IAM permission subset for those who use *TargetGroupBinding* only and don't plan to use the AWS Load Balancer Controller to manage security group rules:*

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



## Network Configuration

Review the [worker nodes security group](https://docs.aws.amazon.com/eks/latest/userguide/sec-group-reqs.html) docs. The node security group must permit incoming traffic on TCP port 9443 from the kubernetes control plane. This is needed for webhook access. 

If you use [eksctl](https://eksctl.io/usage/vpc-networking/), this is the default configuration. 

## Add Controller to Cluster

We recommend using the Helm chart. This supports Fargate and facilitates updating the controller.

=== "Via Helm"

    If you want to run the controller on Fargate, use the Helm chart since it does not depend on the cert-manager.

    ### Detailed Instructions
    Follow the instructions in [aws-load-balancer-controller](https://github.com/aws/eks-charts/tree/master/stable/aws-load-balancer-controller) helm chart.

    ### Summary

    1. Add the EKS chart repo to helm
    ```
    helm repo add eks https://aws.github.io/eks-charts
    ```
    1. Install the TargetGroupBinding CRDs if upgrading the chart via `helm upgrade`.
    ```
    kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"
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



=== "Via YAML manifests"
    ### Install cert-manager

    ```
    kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.5.4/cert-manager.yaml
    ```

    ### Apply YAML
    1. Download spec for load balancer controller.
    ```
    wget https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/download/v2.4.3/v2_4_3_full.yaml
    ```
    1. Edit the saved yaml file, go to the Deployment spec, and set the controller --cluster-name arg value to your EKS cluster name
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
                        - --cluster-name=<INSERT_CLUSTER_NAME>
    ```
    1. If you use IAM roles for service accounts, we recommend that you delete the ServiceAccount from the yaml spec. This will preserve the eksctl created iamserviceaccount if you delete the installation section from the yaml spec.
    ```
    apiVersion: v1
    kind: ServiceAccount
    ```
    1. Apply the yaml file
    ```
    kubectl apply -f v2_4_3_full.yaml
    ```
    1. Optionally download the default ingressclass and ingressclass params
    ```
    wget https://github.com/kubernetes-sigs/aws-load-balancer-controller/releases/download/v2.4.3/v2_4_3_ingclass.yaml
    ```
    1. Apply the ingressclass and params
    ```
    kubectl apply -f v2_4_3_ingclass.yaml
    ```

## Create Update Strategy

The controller doesn't receive security updates automatically. You need to manually upgrade to a newer version when it becomes available.

This can be done using [`helm upgrade`](https://helm.sh/docs/helm/helm_upgrade/) or another strategy to manage the controller deployment.
