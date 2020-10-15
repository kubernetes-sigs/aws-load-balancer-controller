# AWS Load Balancer Controller Installation guide

## Via Helm
Follow the instructions in [aws-load-balancer-controller](https://github.com/aws/eks-charts/tree/master/stable/aws-load-balancer-controller) helm chart.

## Via Yaml manifests

### Migrating from AWS ALB Ingress controller
If AWS ALB Ingress controller is installed, refer to [migrating from v1 to v2](../upgrade/migrate_v1_v2.md)

!!!warning ""
    AWS ALB Ingress controller must be uninstalled before installing AWS Load Balancer controller.

!!! Note
    Existing Ingress resources do not need to be deleted for migration.

### IAM permissions
IAM permissions can either be setup via IAM for ServiceAccount or can be attached directly to the worker node IAM roles.
The controller runs on the worker nodes, so IAM permissions are needed for the controller to access AWS ALB/NLB resources.

#### Setup IAM for ServiceAccount
Step 1. Create IAM OIDC provider
```
eksctl utils associate-iam-oidc-provider \
    --region <region-code> \
    --cluster <your-cluster-name> \
    --approve
```
Step 2. Download IAM policy for the AWS Load Balancer Controller
```
curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v2_ga/docs/install/iam_policy.json
```
Step 3. Create an IAM policy called AWSLoadBalancerControllerIAMPolicy
```
aws iam create-policy \
    --policy-name AWSLoadBalancerControllerIAMPolicy \
    --policy-document file://iam-policy.json
```
Take note of the policy ARN that is returned

Step 4. Create a IAM role and ServiceAccount for the Load Balancer controller, use the ARN from the step above
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
https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v2_ga/docs/install/iam_policy.json
```

#### Upgrading from ALB ingress controller
If migrating from ALB ingress controller, grant [additional IAM permissions](../../install/iam_policy_v1_to_v2_additional.json).

### Install cert-manager
- For Kubernetes 1.16+: `kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.0.2/cert-manager.yaml`
- For Kubernetes <1.16: `kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.0.2/cert-manager-legacy.yaml`

### Download and apply the yaml spec
- curl -o https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v2_ga/config/samples/install_v2_0_0.yaml
- Edit the saved yaml file, go to the Deployment spec, and set the controller --cluster-name arg value to your EKS cluster name
- Apply the yaml file kubectl apply -f install_v2_0_0.yaml
    
!!!note ""
    If you use iamserviceaccount, it is recommended that you delete the ServiceAccount from the yaml spec. Doing so will preserve the eksctl created iamserviceaccount if you delete the installation.

