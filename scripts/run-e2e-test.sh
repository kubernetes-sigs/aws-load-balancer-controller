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

TEST_ID=$(date +%s)
echo "TEST_ID: $TEST_ID"
ROLE_NAME="aws-load-balancer-controller-$TEST_ID"

function cleanUp(){
  # Need to recreae aws-load-balancer controller if we are updating SA
  echo "delete aws-load-balancer-controller if exists"
  helm delete aws-load-balancer-controller -n kube-system --timeout=10m || true
 
  echo "delete service account if exists"
  kubectl delete serviceaccount aws-load-balancer-controller -n kube-system --timeout 10m || true
  
  # IAM role and polcies are AWS Account specific, so need to clean them up if any from previous run
  echo "detach IAM policy if it exists"
  aws iam detach-role-policy --role-name $ROLE_NAME --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy || true

  echo "delete $ROLE_NAME if it exists"
  aws iam delete-role --role-name $ROLE_NAME || true

  # Need to do this as last step
  echo "delete AWSLoadBalancerControllerIAMPolicy if it exists"
  aws iam delete-policy --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy || true
}

echo "cordon off windows nodes"
toggle_windows_scheduling "cordon"

echo "fetch OIDC provider"
OIDC_PROVIDER=$(echo $CLUSTER_INFO |  jq -r '.cluster.identity.oidc.issuer' | sed -e "s/^https:\/\///")
echo "OIDC Provider: $OIDC_PROVIDER"

echo "create IAM policy document file"
cat <<EOF > trust.json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "${OIDC_PROVIDER}:aud": "sts.amazonaws.com",
          "${OIDC_PROVIDER}:sub": "system:serviceaccount:kube-system:aws-load-balancer-controller"
        }
      }
    }
  ]
}
EOF

echo "cleanup any stale resources from previous run"
cleanUp

echo "create Role with above policy document"
aws iam create-role --role-name $ROLE_NAME --assume-role-policy-document file://trust.json --description "IAM Role to be used by aws-load-balancer-controller SA" || true

echo "creating AWSLoadbalancerController IAM Policy"
aws iam create-policy \
    --policy-name AWSLoadBalancerControllerIAMPolicy \
    --policy-document file://"$SCRIPT_DIR"/../docs/install/iam_policy.json || true

echo "attaching AWSLoadbalancerController IAM Policy to $ROLE_NAME"
aws iam attach-role-policy --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy --role-name $ROLE_NAME || true

echo "create service account"
kubectl create serviceaccount aws-load-balancer-controller -n kube-system || true

echo "annotate service account with $ROLE_NAME"
kubectl annotate serviceaccount -n kube-system aws-load-balancer-controller eks.amazonaws.com/role-arn=arn:aws:iam::"$ACCOUNT_ID":role/"$ROLE_NAME" --overwrite=true || true

echo "update helm repo eks"
helm repo add eks https://aws.github.io/eks-charts

helm repo update

echo "Install TargetGroupBinding CRDs"
kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"

echo "Install aws-load-balancer-controller"
helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=$CLUSTER_NAME --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller --set region=$REGION --set vpcId=$VPC_ID

echo_time() {
    date +"%D %T $*"
}

wait_until_deployment_ready() {
	NS=""
	if [ ! -z $2 ]; then
		NS="-n $2"
	fi
	echo_time "Checking if deployment $1 is ready"
	for i in $(seq 1 60); do
		desiredReplicas=$(kubectl get deployments.apps $1 $NS -ojsonpath="{.spec.replicas}")
		availableReplicas=$(kubectl get deployments.apps $1 $NS -ojsonpath="{.status.availableReplicas}")
		if [[ ! -z $desiredReplicas && ! -z $availableReplicas && "$desiredReplicas" -eq "$availableReplicas" ]]; then
			break
		fi
		echo -n "."
		sleep 2
	done 
	echo_time "Deployment $1 $NS replicas desired=$desiredReplicas available=$availableReplicas"
}

wait_until_deployment_ready aws-load-balancer-controller kube-system

function run_ginkgo_test() {
  local focus=$1
  echo "Starting the ginkgo tests from generated ginkgo test binaries with focus: $focus"
  if [ "$IP_FAMILY" == "IPv4" ]; then 
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/ingress.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --ip-family=$IP_FAMILY || true)
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/service.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --ip-family=$IP_FAMILY || true)
  elif [ "$IP_FAMILY" == "IPv6" ]; then
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo $EXTRA_GINKGO_FLAGS --focus="$focus" --skip="instance" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/ingress.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --ip-family=$IP_FAMILY || true)
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo $EXTRA_GINKGO_FLAGS --focus="$focus" --skip="instance" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/service.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --ip-family=$IP_FAMILY || true)
  else
    echo "Invalid IP_FAMILY input, choose from IPv4 or IPv6 only"
  fi
}

#Start the test
run_ginkgo_test

# tail=-1 is added so that no logs are truncated
# https://github.com/kubernetes/kubectl/issues/812
echo "Fetch most recent aws-load-balancer-controller logs"
kubectl logs -l app.kubernetes.io/name=aws-load-balancer-controller --container aws-load-balancer-controller --tail=-1 -n kube-system

echo "Uncordon windows nodes"
toggle_windows_scheduling "uncordon"

echo "clean up resources from current run"
cleanUp

echo "Delete TargetGroupBinding CRDs if exists"
kubectl delete -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master" --timeout=10m || true

echo "Successfully finished the test suite $(($SECONDS / 60)) minutes and $(($SECONDS % 60)) seconds"