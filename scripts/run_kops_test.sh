#!/bin/bash

# This script creates kOps cluster and runs full integration tests
set -euoE pipefail

cleanup(){

  exitcode=$?
  if [[ ${exitcode} -gt 0 ]]; then
    echo "Error occurred. Attempting to clean setup.."
  else
    echo "Tests finished. Attempting to clean setup.."
  fi

  if ! helm::uninstall_aws_lb_controller; then
    echo "Uninstalling AWS LB Controller failed.."
  fi

  if [[ ! -z "${POLICY_ARN}" ]]; then
    if ! iam::detach_policy_from_role "${POLICY_ARN}" "nodes.${CLUSTER_NAME}"; then
      echo "Detaching policy from role failed.."
    fi

    if ! iam::delete_policy "${POLICY_ARN}" "${REGION}"; then
      echo "Delete policy failed.."
    fi
  fi

  if ! kops::delete_kops_cluster; then
    echo "Delete cluster failed.."
  fi
}

: "${TEST_ID:=$RANDOM}"
: "${REGION:=us-west-2}"
: "${KOPS_BINARY_VERSION:=v1.25.3}"
: "${KUBERNETES_VERSION:=1.26.0}"
: "${NODE_SIZE:=c5.xlarge}"
: "${NODE_COUNT:=3}"
: "${OS_OVERRIDE:=linux}"
: "${ENDPOINT:=""}"
: "${SKIP_MAKE_TEST_BINARIES:=false}"
: "${EXTRA_GINKGO_FLAGS:=''}"
: "${FOCUS:=''}"
: "${PRINT_CONTROLLER_LOGS:=false}"

AWS_ACCOUNT_ID=$(aws sts get-caller-identity --region ${REGION} --query Account --output text)
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
GINKGO_TEST_BUILD="$SCRIPT_DIR/../test/build"
CONTROLLER_IAM_POLICY_FILE="$(dirname "${BASH_SOURCE[0]}")/../docs/install/iam_policy.json"
CONTROLLER_IAM_POLICY_NAME="aws-lb-controller-kops-${TEST_ID}"
KOPS_S3_BUCKET="kops-alb-eks-${AWS_ACCOUNT_ID}"
KOPS_STATE_STORE=s3://$KOPS_S3_BUCKET
CLUSTER_NAME="kops-alb-test-cluster-${TEST_ID}.k8s.local"
KOPS_BIN=~/kops_bin
SSH_KEYS=~/.ssh/kopstempkey
USE_EKSCTL=false
POLICY_ARN=""

# Import Helper Functions
source "$SCRIPT_DIR"/lib/common.sh
source "$SCRIPT_DIR"/lib/kops.sh
source "$SCRIPT_DIR"/lib/iam.sh
source "$SCRIPT_DIR"/lib/helm.sh
source "$SCRIPT_DIR"/lib/run_tests.sh

mkdir -p $KOPS_BIN

if [[ $REGION == "cn-north-1" ]];then
  IMAGE="918309763551.dkr.ecr.cn-north-1.amazonaws.com.cn/amazon/aws-load-balancer-controller"
elif [[ $REGION == "cn-northwest-1" ]];then
  IMAGE="961992271922.dkr.ecr.cn-northwest-1.amazonaws.com.cn/amazon/aws-load-balancer-controller"
else
  IMAGE="602401143452.dkr.ecr.us-west-2.amazonaws.com/amazon/aws-load-balancer-controller"
fi

trap cleanup EXIT

main(){

  # Step 1: Create kOps cluster and wait for it to complete
  kops::create_kops_cluster

  # Step 2: Create policy and attach to nodes role created by kOps
  POLICY_ARN=$(iam::create_policy "${CONTROLLER_IAM_POLICY_NAME}" "${CONTROLLER_IAM_POLICY_FILE}" "${REGION}")
  iam::attach_policy_to_role "${POLICY_ARN}" "nodes.${CLUSTER_NAME}"

  # Step 3: Set hop limit to 2 for IMDSv2
  INSTANCE_IDS=$(kubectl get nodes -o json | jq -r '.items[].metadata.name')
  for ID in $INSTANCE_IDS
  do
    aws ec2 modify-instance-metadata-options --http-put-response-hop-limit 2 --region $REGION --instance-id $ID >/dev/null 2>&1
  done

  # Step 4: Get cluster metadata
  INSTANCE_ID=$(kubectl get nodes -l node-role.kubernetes.io/control-plane -o json | jq -r '.items[0].metadata.name')
  VPC_ID=$(aws ec2 describe-instances --instance-ids "${INSTANCE_ID}" --no-cli-pager | jq -r '.Reservations[].Instances[].VpcId')

  # Step 5: Install controller and wait to become ready
  helm::install_aws_lb_controller ${VPC_ID} ${IMAGE} true

  # Step 6: Run Ginkgo tests
  ginkgo::run_test ${EXTRA_GINKGO_FLAGS} ${FOCUS} ${VPC_ID}
}

main