# Load Balancer Controller Installation

!!!warning "Existing AWS ALB Ingress Controller users"
    AWS ALB Ingress controller must be uninstalled before installing AWS Load Balancer controller.
    Please follow our [migration guide](upgrade/migrate_v1_v2.md) to do migration.

!!!note "Security updates"
    The controller doesn't receive security updates automatically. You need to manually upgrade to a newer version when it becomes available.

## IAM Permissions

#### Setup IAM role for service accounts
The controller runs on the worker nodes, so it needs access to the AWS ALB/NLB resources via IAM permissions. 
The IAM permissions can either be setup via IAM roles for ServiceAccount or can be attached directly to the worker node IAM roles.

1. Create IAM OIDC provider
    ```
    eksctl utils associate-iam-oidc-provider \
        --region <region-code> \
        --cluster <your-cluster-name> \
        --approve
    ```

1. Download IAM policy for the AWS Load Balancer Controller
    ```
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.1.0/docs/install/iam_policy.json
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
    --approve
    ```
Setup IAM manually
If not setting up IAM for ServiceAccount, apply the IAM policies from the following URL at minimum.
```
curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.1.0/docs/install/iam_policy.json
```
## Add Controller to Cluster

=== "Via Helm"
    ### Detailed Instructions 
    Follow the instructions in [aws-load-balancer-controller](https://github.com/aws/eks-charts/tree/master/stable/aws-load-balancer-controller) helm chart.

    ### Summary

    1. Add the EKS chart repo to helm
    ```
    helm repo add eks https://aws.github.io/eks-charts
    ```
    1. Install the TargetGroupBinding CRDs
    ```
    kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"
    ```
    1. Install the helm chart
    ```
    helm install eks/aws-load-balancer-controller
    ```

    

=== "Via YAML manifests"
    ### Install cert-manager
    - For Kubernetes 1.16+: 
    ```
    kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.0.2/cert-manager.yaml
    ```
    - For Kubernetes <1.16: 
    ```
    kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.0.2/cert-manager-legacy.yaml
    ```
    
    ### Apply YAML
    1. Download spec for load balancer controller. 
    ```
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.1.0/docs/install/v2_1_0_full.yaml
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
    kubectl apply -f v2_1_0_full.yaml
    ```
    
    
