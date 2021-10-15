# Load Balancer Controller Installation

## Kubernetes version requirements
* AWS Load Balancer Controller v2.0.0~v2.1.3 requires Kubernetes 1.15+
* AWS Load Balancer Controller v2.2.0+ requires Kubernetes 1.16+

!!!warning "Existing AWS ALB Ingress Controller users"
    AWS ALB Ingress controller must be uninstalled before installing AWS Load Balancer controller.
    Please follow our [migration guide](upgrade/migrate_v1_v2.md) to do migration.

!!!note "Security updates"
    The controller doesn't receive security updates automatically. You need to manually upgrade to a newer version when it becomes available.

!!!note "non-EKS cluster"
    You can run the controller on a non-EKS cluster, for example kops or vanilla k8s. Here are the things to consider -

    - In lieu of IAM for service account, you will have to manually attach the IAM permissions to your worker nodes IAM roles
    - Ensure subnets are tagged appropriately for auto-discovery to work
    - For IP targets, pods must have IPs from the VPC subnets. You can configure `amazon-vpc-cni-k8s` plugin for this purpose.

## Using metadata server version 2 (IMDSv2)
If you are using the IMDSv2 you must set the hop limit to 2 or higher in order to allow the AWS Load Balancer Controller to perform the metadata introspection. Otherwise you have to manually specify the AWS region and the VPC via the controller flags `--aws-region` and `--aws-vpc-id`.


!!!tip 
    You can set the IMDSv2 hop limit as follows:
    ```
    aws ec2 modify-instance-metadata-options --http-put-response-hop-limit 2 --region <region> --instance-id <instance-id>
    ```

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
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.2.1/docs/install/iam_policy.json
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
#### Setup IAM manually
If not setting up IAM for ServiceAccount, apply the IAM policies from the following URL at minimum.
```
curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.2.1/docs/install/iam_policy.json
```
## Add Controller to Cluster

!!!note "Use Fargate"
    If you want to run the controller on Fargate, use Helm chart since it does not depend on the cert-manager.

=== "Via Helm" 

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

        !!!tip
            Only run one of the two following `helm install` commands depending on whether or not your cluster uses IAM roles for service accounts.

    1. Install the helm chart if using IAM roles for service accounts. **NOTE** you need to specify both of the chart values `serviceAccount.create=false` and `serviceAccount.name=aws-load-balancer-controller`
    ```
    helm install aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=<cluster-name> --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller
    ```
    1. Install the helm chart if **not** using IAM roles for service accounts
    ```
    helm install aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=<cluster-name>
    ```

    

=== "Via YAML manifests"
    ### Configure SSL
    In order to use the provided webhooks, SSL must be configured on each API endpoint. 

    #### Via cert-manager
	The quickest solution would be to provision certificates via cert-manager:
    ```
    kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v1.5.3/cert-manager.yaml
    ```

    #### Install manually
    It is also possible to create the certificates manually by creating a self-signed cert:
    - Create a self-signed certificate authority.
	- Create a certificate request using the internal DNS names of the webhook services:
    ```
    aws-load-balancer-webhook-service.kube-system.svc
    aws-load-balancer-webhook-service.kube-system.svc.cluster.local
    ```
    - Create a self-signed certificate from the above certificate authority for the above request.

	Once the certs have been created, you need to make them available to the cluster. This can be done using a Kubernetes Secret:
    - Create the required `aws-load-balancer-webhook-tls` secret in the `kube-system` namespace:
    ```
    kubectl create secret --namespace=kube-system tls aws-load-balancer-webhook-tls \
      --cert=path/to/cert/file \
      --key=path/to/key/file
    ```

    Finally, you will need to update the `clientConfig` for the reuired webhook configurations to include the CA Bundle:
    ```
    clientConfig:
      caBundle: "Ci0tLS0tQk..." # PEM encoded CA bundle, created earlier, which will be used to validate the webhook's server certificate.
    ```
    
    ### Apply YAML
    1. Download spec for load balancer controller. 
    ```
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.2.1/docs/install/v2_2_1_full.yaml
    ```

    !!!note "Manual TLS"
        If you are not using cert-manager, remove related resources and annotations before applying.

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
    kubectl apply -f v2_2_1_full.yaml
    ```
    
    
