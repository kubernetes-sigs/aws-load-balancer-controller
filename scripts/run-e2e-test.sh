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

function set_Put_Response_Hop_Limit(){
  limit=$1
  linux_nodes=$(kubectl get nodes -l kubernetes.io/os=linux | tail -n +2 | cut -d' ' -f1)
  dns_name=""
  for n in $linux_nodes
  do
    if [ -z "$dns_name" ];then
      dns_name="$n"
    else
      dns_name="$dns_name,$n"
    fi
  done
  instance_ids=$(aws ec2 describe-instances --filters Name=private-dns-name,Values=$dns_name | jq -r '.Reservations[].Instances[].InstanceId')
  for id in $instance_ids
  do
    aws ec2 modify-instance-metadata-options --instance-id $id --http-put-response-hop-limit $limit
  done
}

echo "Cordon off windows nodes"
toggle_windows_scheduling "cordon"

# This is needed so that EC2 IMDS can be accessed from aws-load-balancer-controller container
echo "Set HttpPutResponseHopLimit to 2"
set_Put_Response_Hop_Limit 2

echo "Fetching NodeInstanceRole"
INSTANCE_PROFILE=$(aws iam list-instance-profiles | jq -r '.InstanceProfiles[].InstanceProfileName' | grep '.*networking-prow.*linux.*-NodeInstanceProfile.*')
ROLE_NAME=$(aws iam get-instance-profile --instance-profile-name $INSTANCE_PROFILE | jq -r '.InstanceProfile.Roles[] | .RoleName')
echo "NodeInstanceRole: $ROLE_NAME"

echo "Creating AWSLoadbalancerController IAM Policy"
aws iam create-policy \
    --policy-name AWSLoadBalancerControllerIAMPolicy \
    --policy-document file://"$SCRIPT_DIR"/../docs/install/iam_policy.json || true

echo "Attaching AWSLoadbalancerController IAM Policy to NodeInstanceRole"
aws iam attach-role-policy --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy --role-name $ROLE_NAME || true

echo "Update helm repo eks"
helm repo add eks https://aws.github.io/eks-charts

helm repo update

echo "Install TargetGroupBinding CRDs"
kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"

echo "Install aws-load-balancer-controller"
helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=$CLUSTER_NAME --set serviceAccount.create=false --set region=$REGION --set vpcId=$VPC_ID

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

echo "Delete aws-load-balancer-controller"
helm delete aws-load-balancer-controller -n kube-system --timeout=10m || true

echo "Delete TargetGroupBinding CRDs"
kubectl delete -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master" --timeout=10m || true

echo "Uncordon windows nodes"
toggle_windows_scheduling "uncordon"

echo "Detach IAM policy"
aws iam detach-role-policy --role-name $ROLE_NAME --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy || true

# Need to do this as last step
echo "Delete IAM Policy"
aws iam delete-policy --policy-arn arn:aws:iam::$ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy || true

echo "Successfully finished the test suite $(($SECONDS / 60)) minutes and $(($SECONDS % 60)) seconds"