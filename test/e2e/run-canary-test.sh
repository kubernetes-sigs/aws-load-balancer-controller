#!/bin/bash

# This script runs e2e tests on the AWS Load Balancer Controller

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
echo "Running AWS Load Balancer Controller e2e tests with the following variables
KUBE CONFIG: $KUBE_CONFIG_PATH
CLUSTER_NAME: $CLUSTER_NAME
REGION: $REGION
OS_OVERRIDE: $OS_OVERRIDE"

if [[ -z "${OS_OVERRIDE}" ]]; then
  OS_OVERRIDE=linux
fi

GET_CLUSTER_INFO_CMD="aws eks describe-cluster --name $CLUSTER_NAME --region $REGION"

if [[ -z "${ENDPOINT}" ]]; then
  CLUSTER_INFO=$($GET_CLUSTER_INFO_CMD)
else
  CLUSTER_INFO=$($GET_CLUSTER_INFO_CMD --endpoint $ENDPOINT)
fi

VPC_ID=$(echo $CLUSTER_INFO | jq -r '.cluster.resourcesVpcConfig.vpcId')
SERVICE_ROLE_ARN=$(echo $CLUSTER_INFO | jq -r '.cluster.roleArn')
ROLE_NAME=${SERVICE_ROLE_ARN##*/}
ACCOUNT_ID=$(aws sts get-caller-identity | jq -r '.Account')

echo "VPC ID: $VPC_ID, Service Role ARN: $SERVICE_ROLE_ARN, Role Name: $ROLE_NAME"

eksctl utils associate-iam-oidc-provider \
    --region $REGION \
    --cluster $CLUSTER_NAME \
    --approve

echo "Creating AWSLoadbalancerController IAM Policy"
curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.2.1/docs/install/iam_policy.json

aws iam create-policy \
    --policy-name AWSLoadBalancerControllerIAMPolicy \
    --policy-document file://iam-policy.json || true

echo "Creating IAM serviceaccount"
eksctl create iamserviceaccount \
--cluster=$CLUSTER_NAME \
--namespace=kube-system \
--name=aws-load-balancer-controller \
--attach-policy-arn=arn:aws:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy \
--override-existing-serviceaccounts \
--approve || true

echo "Update helm repo eks"
helm repo add eks https://aws.github.io/eks-charts

helm repo update

echo "Install TargetGroupBinding CRDs"
kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"

echo "Install aws-load-balacner-controller"
helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=$CLUSTER_NAME --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller

#Start the test
echo "Starting the ginkgo test suite" 

(cd $SCRIPT_DIR && CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo -v -r -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID || true)

echo "Delete aws-load-balacner-controller"
helm delete aws-load-balancer-controller -n kube-system

echo "Delete iamserviceaccount"
eksctl delete iamserviceaccount --name aws-load-balancer-controller --namespace kube-system --cluster $CLUSTER_NAME || true

echo "Delete TargetGroupBinding CRDs"
kubectl delete -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"

echo "Successfully finished the test suite"