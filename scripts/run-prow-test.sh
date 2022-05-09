#!/bin/bash

# This script runs e2e tests on the AWS Load Balancer Controller

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
GINKGO_TEST_BUILD="$SCRIPT_DIR/../test/build"

source "$SCRIPT_DIR"/lib/common.sh

function toggle_windows_scheduling(){
  schedule=$1
  nodes=$(kubectl get nodes -l kubernetes.io/os=windows | tail -n +2 | cut -d' ' -f1)
  for n in $nodes
  do
    kubectl $schedule $n
  done
}

echo "Cordon off windows nodes"
toggle_windows_scheduling "cordon"

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

echo "Install aws-load-balancer-controller"
helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=$CLUSTER_NAME --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller --set region=$REGION --set vpcId=$VPC_ID

function run_ginkgo_test() {
  local focus=$1
  echo "Starting the ginkgo tests from generated ginkgo test binaries with focus: $focus" 
  (CGO_ENABLED=0 ginkgo $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 40m --fail-on-pending $GINKGO_TEST_BUILD/ingress.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID || true)
  (CGO_ENABLED=0 ginkgo $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 40m --fail-on-pending $GINKGO_TEST_BUILD/service.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID || true)
}

#Start the test
run_ginkgo_test

# tail=-1 is added so that no logs are truncated
# https://github.com/kubernetes/kubectl/issues/812
echo "Fetch most recent aws-load-balancer-controller logs"
kubectl logs -l app.kubernetes.io/name=aws-load-balancer-controller --container aws-load-balancer-controller --tail=-1 -n kube-system

echo "Delete aws-load-balancer-controller"
helm delete aws-load-balancer-controller -n kube-system --timeout=10m || true

echo "Delete iamserviceaccount"
eksctl delete iamserviceaccount --name aws-load-balancer-controller --namespace kube-system --cluster $CLUSTER_NAME --timeout=10m || true

echo "Delete TargetGroupBinding CRDs"
kubectl delete -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master" --timeout=10m || true

echo "Uncordon windows nodes"
toggle_windows_scheduling "uncordon"

echo "Successfully finished the test suite $(($SECONDS / 60)) minutes and $(($SECONDS % 60)) seconds"