# Setup ALB ingress controller
This document describes how to install ALB ingress controller into your kubernetes cluster on AWS.
If you'd prefer an end-to-end walkthrough of setup instead, see [the echoservice walkthrough](../walkthrough/echoserver.md)

## Prerequisites
This section details what must be setup in order for the controller to run.

### Kubelet
The [kubelet](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/) must be run with `--cloud-provider=aws`. This populates the EC2 instance ID in each node's spec.

### Role Permissions
Adequate roles and policies must be configured in AWS and available to the node(s) running the controller. How access is granted is up to you. Some will attach the needed rights to node's role in AWS. Others will use projects like [kube2iam](https://github.com/jtblin/kube2iam).

An example policy with the minimum rights can be found at [iam-policy.json](../../examples/iam-policy.json).

## Installation
You can choose to install ALB ingress controller via Helm or Kubectl
### Helm
1. Add helm incubator repository
    ```bash
    helm repo add incubator http://storage.googleapis.com/kubernetes-charts-incubator
    ```

2. Install ALB ingress controller

    ``` bash
    helm install incubator/aws-alb-ingress-controller --set autoDiscoverAwsRegion=true --set autoDiscoverAwsVpcID=true --set clusterName=MyClusterName
    ```

More docs on [hub.helm.sh](https://hub.helm.sh/charts/incubator/aws-alb-ingress-controller)

### Kubectl
1. Download sample ALB ingress controller manifest
    ``` bash
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.5/docs/examples/alb-ingress-controller.yaml
    ```

2. Configure the ALB ingress controller manifest

    At minimum, edit the following variables:

    -  `--cluster-name=devCluster`:  name of the cluster. AWS resources will be tagged with `kubernetes.io/cluster/devCluster:owned`

    !!!tip
        If [ec2metadata](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html) is unavailable from the controller pod, edit the following variables:

        -  `--aws-vpc-id=vpc-xxxxxx`: vpc ID of the cluster.
        -  `--aws-region=us-west-1`: AWS region of the cluster.

3. Deploy the RBAC roles manifest

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.5/docs/examples/rbac-role.yaml
    ```

4. Deploy the ALB ingress controller manifest

    ```bash
    kubectl apply -f alb-ingress-controller.yaml
    ```

5. Verify the deployment was successful and the controller started

    ```bash
    kubectl logs -n kube-system $(kubectl get po -n kube-system | egrep -o "alb-ingress[a-zA-Z0-9-]+")
    ```

    Should display output similar to the following.

    ```console
    -------------------------------------------------------------------------------
    AWS ALB Ingress controller
    Release:    1.0.0
    Build:      git-7bc1850b
    Repository: https://github.com/kubernetes-sigs/aws-alb-ingress-controller.git
    -------------------------------------------------------------------------------
    ```
