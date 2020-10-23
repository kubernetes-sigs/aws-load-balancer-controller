# AWS Load Balancer Controller Installation guide

!!!warning "Existing AWS ALB Ingress Controller users"
    AWS ALB Ingress controller must be uninstalled before installing AWS Load Balancer controller.
    Please follow our [migration guide](../upgrade/migrate_v1_v2.md) to do migration.

=== "Via Helm"
    Follow the instructions in [aws-load-balancer-controller](https://github.com/aws/eks-charts/tree/master/stable/aws-load-balancer-controller) helm chart.

=== "Via YAML manifests"
    ### IAM Permissions
    The controller runs on the worker nodes, so it needs access to the AWS ALB/NLB resources via IAM permissions. 
    The IAM permissions can either be setup via IAM roles for ServiceAccount or can be attached directly to the worker node IAM roles.
    
    #### Setup IAM role for service accounts
    1. Create IAM OIDC provider
        ```
        eksctl utils associate-iam-oidc-provider \
            --region <region-code> \
            --cluster <your-cluster-name> \
            --approve
        ```
    
    1. Download IAM policy for the AWS Load Balancer Controller
        ```
        curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/main/docs/install/iam_policy.json
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
    #### Setup IAM manually
    If not setting up IAM for ServiceAccount, apply the IAM policies from the following URL at minimum.
    ```
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/main/docs/install/iam_policy.json
    ```
    
    ### Install cert-manager
    - For Kubernetes 1.16+: `kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.0.2/cert-manager.yaml`
    - For Kubernetes <1.16: `kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.0.2/cert-manager-legacy.yaml`
    
    ### Download and apply the yaml spec
    - https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/main/docs/install/v2_0_0_full.yaml
    - Edit the saved yaml file, go to the Deployment spec, and set the controller --cluster-name arg value to your EKS cluster name
    - Apply the yaml file kubectl apply -f install_v2_0_0.yaml
    
    !!!note ""
        If you use IAM roles for service accounts, we recommend that you delete the ServiceAccount from the yaml spec. Doing so will preserve the eksctl created iamserviceaccount if you delete the installation.
