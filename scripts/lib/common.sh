#!/bin/bash

# Helper script used for running prow tests for aws-load-balancer-controller

SECONDS=0

# Set IP Family
IP_FAMILY="${IP_FAMILY:-IPv4}"

echo "Running AWS Load Balancer Controller e2e tests with the following variables
KUBE_CONFIG_PATH: $KUBE_CONFIG_PATH
CLUSTER_NAME: $CLUSTER_NAME
REGION: $REGION
IP_FAMILY: $IP_FAMILY
OS_OVERRIDE: $OS_OVERRIDE
ENDPOINT: $ENDPOINT"

if [[ -z "${OS_OVERRIDE}" ]]; then
  OS_OVERRIDE=linux
fi

if [[ -z "${SKIP_MAKE_TEST_BINARIES}" ]]; then
  echo "making ginkgo test binaries"
  (cd $SCRIPT_DIR/../test && make build-test-binaries)
else
  echo "skipping making ginkgo test binaries"
fi

# Request timesout in China Regions with default proxy
if [[ $REGION == "cn-north-1" || $REGION == "cn-northwest-1" ]]; then
  go env -w GOPROXY=https://goproxy.cn,direct
  go env -w GOSUMDB=sum.golang.google.cn
fi

GET_CLUSTER_INFO_CMD="aws eks describe-cluster --name $CLUSTER_NAME --region $REGION"

if [[ -z "${ENDPOINT}" ]]; then
  CLUSTER_INFO=$($GET_CLUSTER_INFO_CMD)
else
  CLUSTER_INFO=$($GET_CLUSTER_INFO_CMD --endpoint $ENDPOINT)
fi

VPC_ID=$(echo $CLUSTER_INFO | jq -r '.cluster.resourcesVpcConfig.vpcId')
ACCOUNT_ID=$(aws sts get-caller-identity | jq -r '.Account')

echo "VPC ID: $VPC_ID"
