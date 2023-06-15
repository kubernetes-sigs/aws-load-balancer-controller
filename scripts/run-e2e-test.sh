#!/bin/bash

# This script runs e2e tests on the AWS Load Balancer Controller

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
GINKGO_TEST_BUILD="$SCRIPT_DIR/../test/build"

source "$SCRIPT_DIR"/lib/common.sh

# TEST_IMAGE_REGISTRY is the registry in test-infra-* accounts where e2e test images are stored
TEST_IMAGE_REGISTRY=${TEST_IMAGE_REGISTRY:-"617930562442.dkr.ecr.us-west-2.amazonaws.com"}

# PROD_IMAGE_REGISTRY is the registry in build-prod-* accounts where prod LBC images are stored
PROD_IMAGE_REGISTRY=${PROD_IMAGE_REGISTRY:-"602401143452.dkr.ecr.us-west-2.amazonaws.com"}

ADC_REGIONS="us-iso-east-1 us-isob-east-1 us-iso-west-1"

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
  aws iam detach-role-policy --role-name $ROLE_NAME --policy-arn arn:${AWS_PARTITION}:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy || true

  # wait for 10 sec to complete detaching of IAM policy
  sleep 10

  echo "delete $ROLE_NAME if it exists"
  aws iam delete-role --role-name $ROLE_NAME || true

  # Need to do this as last step
  echo "delete AWSLoadBalancerControllerIAMPolicy if it exists"
  aws iam delete-policy --policy-arn arn:${AWS_PARTITION}:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy || true
}

echo "cordon off windows nodes"
toggle_windows_scheduling "cordon"

echo "fetch OIDC provider"
OIDC_PROVIDER=$(echo $CLUSTER_INFO |  jq -r '.cluster.identity.oidc.issuer' | sed -e "s/^https:\/\///")
echo "OIDC Provider: $OIDC_PROVIDER"

# get the aws partition and iam policy file name based on regions
AWS_PARTITION="aws"
IAM_POLCIY_FILE="iam_policy.json"

if [[ $REGION == "cn-north-1" || $REGION == "cn-northwest-1" ]];then
  AWS_PARTITION="aws-cn"
  IAM_POLCIY_FILE="iam_policy_cn.json"
else if [[ $ADC_REGIONS == *"$REGION"* ]]; then
  if [[ $REGION == "us-isob-east-1" ]]; then
    AWS_PARTITION="aws-iso-b"
    IAM_POLCIY_FILE="iam_policy_isob.json"
  else
    AWS_PARTITION="aws-iso"
    IAM_POLCIY_FILE="iam_policy_iso.json"
  fi
fi
fi
echo "AWS_PARTITION $AWS_PARTITION"
echo "IAM_POLCIY_FILE $IAM_POLCIY_FILE"

IMAGE="$PROD_IMAGE_REGISTRY/amazon/aws-load-balancer-controller"
echo "IMAGE: $IMAGE"
echo "create IAM policy document file"
cat <<EOF > trust.json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:${AWS_PARTITION}:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
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
    --policy-document file://"$SCRIPT_DIR"/../docs/install/${IAM_POLCIY_FILE} || true

echo "attaching AWSLoadbalancerController IAM Policy to $ROLE_NAME"
aws iam attach-role-policy --policy-arn arn:${AWS_PARTITION}:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy --role-name $ROLE_NAME || true

echo "create service account"
kubectl create serviceaccount aws-load-balancer-controller -n kube-system || true

echo "annotate service account with $ROLE_NAME"
kubectl annotate serviceaccount -n kube-system aws-load-balancer-controller eks.amazonaws.com/role-arn=arn:${AWS_PARTITION}:iam::"$ACCOUNT_ID":role/"$ROLE_NAME" --overwrite=true || true

echo "update helm repo eks"
# for ADC regions, install chart from local path
if [[ $ADC_REGIONS == *"$REGION"* ]]; then
  echo "Helm install from local chart path"
  helm upgrade -i aws-load-balancer-controller ../helm/aws-load-balancer-controller -n kube-system --set clusterName=$CLUSTER_NAME --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller --set region=$REGION --set vpcId=$VPC_ID --set image.repository=$IMAGE
else
  echo "Update helm repo from github"
  helm repo add eks https://aws.github.io/eks-charts
  helm repo update
  echo "Install aws-load-balancer-controller"
  helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=$CLUSTER_NAME --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller --set region=$REGION --set vpcId=$VPC_ID --set image.repository=$IMAGE
fi

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
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo --no-color $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/ingress.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --test-image-registry=$TEST_IMAGE_REGISTRY --ip-family=$IP_FAMILY || true)
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo --no-color $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/service.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --test-image-registry=$TEST_IMAGE_REGISTRY --ip-family=$IP_FAMILY || true)
  elif [ "$IP_FAMILY" == "IPv6" ]; then
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo --no-color $EXTRA_GINKGO_FLAGS --focus="$focus" --skip="instance" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/ingress.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --test-image-registry=$TEST_IMAGE_REGISTRY --ip-family=$IP_FAMILY || true)
    (CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo --no-color $EXTRA_GINKGO_FLAGS --focus="$focus" --skip="instance" -v --timeout 60m --fail-on-pending $GINKGO_TEST_BUILD/service.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --test-image-registry=$TEST_IMAGE_REGISTRY --ip-family=$IP_FAMILY || true)
  else
    echo "Invalid IP_FAMILY input, choose from IPv4 or IPv6 only"
  fi
}

#Start the test
run_ginkgo_test

# tail=-1 is added so that no logs are truncated
# https://github.com/kubernetes/kubectl/issues/812
echo "Fetch most recent aws-load-balancer-controller logs"
kubectl logs -l app.kubernetes.io/name=aws-load-balancer-controller --container aws-load-balancer-controller --tail=-1 -n kube-system || true

echo "Uncordon windows nodes"
toggle_windows_scheduling "uncordon"

echo "clean up resources from current run"
cleanUp

echo "Delete CRDs if exists"
if [[ $ADC_REGIONS == *"$REGION"* ]]; then
  kubectl delete -k "../helm/aws-load-balancer-controller/crds" --timeout=30m || true
else
  kubectl delete -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master" --timeout=30m || true
fi
echo "Successfully finished the test suite $(($SECONDS / 60)) minutes and $(($SECONDS % 60)) seconds"
