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
CONTAINER_NAME="aws-load-balancer-controller"
: "${DISABLE_WAFV2:=false}"
DISABLE_WAFV2_FLAGS=""

function toggle_windows_scheduling(){
  schedule=$1
  nodes=$(kubectl get nodes -l kubernetes.io/os=windows | tail -n +2 | cut -d' ' -f1)
  for n in $nodes
  do
    kubectl $schedule $n
  done
}

TEST_ID=$(date +%s)-$((RANDOM % 1000))
echo "TEST_ID: $TEST_ID"
ROLE_NAME="aws-load-balancer-controller-$TEST_ID"
POLICY_NAME="AWSLoadBalancerControllerIAMPolicy-$TEST_ID"

function cleanUp(){
  echo "delete serviceaccount"
  kubectl delete serviceaccount aws-load-balancer-controller -n kube-system --timeout 60s || true

  echo "detach IAM policy"
  aws iam detach-role-policy --role-name $ROLE_NAME --policy-arn arn:${AWS_PARTITION}:iam::$ACCOUNT_ID:policy/$POLICY_NAME || true

  # wait for 10 sec to complete detaching of IAM policy
  sleep 10

  echo "delete $ROLE_NAME"
  aws iam delete-role --role-name $ROLE_NAME || true

  echo "delete $POLICY_NAME"
  aws iam delete-policy --policy-arn arn:${AWS_PARTITION}:iam::$ACCOUNT_ID:policy/$POLICY_NAME || true

  echo "Delete CRDs if exists"
  if [[ $ADC_REGIONS == *"$REGION"* ]]; then
    kubectl delete -k "$SCRIPT_DIR"/../helm/aws-load-balancer-controller/crds --timeout=30s || true
  else
    kubectl delete -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master" --timeout=30s || true
  fi
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
elif [[ $ADC_REGIONS == *"$REGION"* ]]; then
  if [[ $REGION == "us-isob-east-1" ]]; then
    AWS_PARTITION="aws-iso-b"
    IAM_POLCIY_FILE="iam_policy_isob.json"
  else
    AWS_PARTITION="aws-iso"
    IAM_POLCIY_FILE="iam_policy_iso.json"
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

echo "create Role with above policy document"
aws iam create-role --role-name $ROLE_NAME --assume-role-policy-document file://trust.json --description "IAM Role to be used by aws-load-balancer-controller SA" || true

echo "creating AWSLoadbalancerController IAM Policy"
aws iam create-policy \
    --policy-name $POLICY_NAME \
    --policy-document file://"$SCRIPT_DIR"/../docs/install/${IAM_POLCIY_FILE} || true

echo "attaching AWSLoadBalancerController IAM Policy to $ROLE_NAME"
aws iam attach-role-policy --policy-arn arn:${AWS_PARTITION}:iam::$ACCOUNT_ID:policy/$POLICY_NAME --role-name $ROLE_NAME || true

echo "create service account"
kubectl create serviceaccount aws-load-balancer-controller -n kube-system || true

echo "annotate service account with $ROLE_NAME"
kubectl annotate serviceaccount -n kube-system aws-load-balancer-controller eks.amazonaws.com/role-arn=arn:${AWS_PARTITION}:iam::"$ACCOUNT_ID":role/"$ROLE_NAME" --overwrite=true || true

function install_controller_for_adc_regions() {
    echo "install cert-manager"
    cert_manager_yaml="$SCRIPT_DIR"/../test/prow/cert_manager.yaml

    # replace the url to the test images registry in ADC regions
    declare -A url_mapping
    url_mapping["quay.io/jetstack/cert-manager-cainjector"]="$TEST_IMAGE_REGISTRY/networking-e2e-test-images/cert-manager-cainjector"
    url_mapping["quay.io/jetstack/cert-manager-controller"]="$TEST_IMAGE_REGISTRY/networking-e2e-test-images/cert-manager-controller"
    url_mapping["quay.io/jetstack/cert-manager-webhook"]="$TEST_IMAGE_REGISTRY/networking-e2e-test-images/cert-manager-webhook"
    # Iterate through the mapping and perform the replacements
    for default_url in "${!url_mapping[@]}"; do
      adc_url="${url_mapping[$default_url]}"
      sed -i "s#$default_url#$adc_url#g" "$cert_manager_yaml"
    done
    echo "Image URLs in $cert_manager_yaml have been updated to use the ADC registry"
    kubectl apply -f $cert_manager_yaml || true
    sleep 60s
    echo "install the controller via yaml"
    controller_yaml="$SCRIPT_DIR"/../test/prow/v2_6_0_adc.yaml
    default_controller_image="public.ecr.aws/eks/aws-load-balancer-controller"
    sed -i "s#$default_controller_image#$IMAGE#g" "$controller_yaml"
    echo "Image URL in $controller_yaml has been updated to $IMAGE"
    sed -i "s#your-cluster-name#$CLUSTER_NAME#g" "$controller_yaml"
    echo "cluster name in $controller_yaml has been update to $CLUSTER_NAME"
    kubectl apply -f $controller_yaml || true
    kubectl rollout status -n kube-system deployment aws-load-balancer-controller || true

    echo "apply the manifest for ingressclass and ingressclassparam"
    ingclass_yaml="$SCRIPT_DIR"/../test/prow/v2_6_0_ingclass.yaml
    kubectl apply -f $ingclass_yaml || true
}

function enable_primary_ipv6_address() {
  echo "enable primary ipv6 address for the ec2 instance"
  ENI_IDS=$(aws ec2 describe-instances --region $REGION --filters "Name=tag:aws:eks:cluster-name,Values=$CLUSTER_NAME" --query "Reservations[].Instances[].NetworkInterfaces[].NetworkInterfaceId" --output text)
  ENI_COUNT=$(echo "$ENI_IDS" | wc -w)
  echo "found $ENI_COUNT ENIs: $ENI_IDS"
  for ENI_ID in $ENI_IDS; do
    echo "enable primary ipv6 address for ENI $ENI_ID"
    aws ec2 modify-network-interface-attribute --region $REGION --network-interface-id $ENI_ID --enable-primary-ipv6
  done
}

echo "installing AWS load balancer controller"
if [[ $ADC_REGIONS == *"$REGION"* ]]; then
  echo "for ADC regions, install via manifest"
  CONTAINER_NAME="controller"
  install_controller_for_adc_regions
  echo "disable NLB Security Group and Listener Rules tagging as they are not supported in ADC yet"
  kubectl patch deployment aws-load-balancer-controller -n kube-system \
    --type=json \
    -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--feature-gates=NLBSecurityGroup=false,ListenerRulesTagging=false"}]' || true
else
  echo "install via helm repo, update helm repo from github"
  if [[ "$DISABLE_WAFV2" == true ]]; then
    DISABLE_WAFV2_FLAGS="--set enableShield=false --set enableWaf=false --set enableWafv2=false"
  fi
  helm repo add eks https://aws.github.io/eks-charts
  helm repo update
  echo "Install aws-load-balancer-controller"
  helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=$CLUSTER_NAME --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller --set region=$REGION --set vpcId=$VPC_ID --set image.repository=$IMAGE $DISABLE_WAFV2_FLAGS
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
  TEST_RESULT=success
  local focus=$1
  echo "Starting the ginkgo tests from generated ginkgo test binaries with focus: $focus"
  if [ "$IP_FAMILY" == "IPv4" ] || [ "$IP_FAMILY" == "IPv6" ]; then
    CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo --no-color $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 2h --fail-on-pending $GINKGO_TEST_BUILD/ingress.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --test-image-registry=$TEST_IMAGE_REGISTRY --ip-family=$IP_FAMILY || TEST_RESULT=fail
    CGO_ENABLED=0 GOOS=$OS_OVERRIDE ginkgo --no-color $EXTRA_GINKGO_FLAGS --focus="$focus" -v --timeout 2h --fail-on-pending $GINKGO_TEST_BUILD/service.test -- --kubeconfig=$KUBE_CONFIG_PATH --cluster-name=$CLUSTER_NAME --aws-region=$REGION --aws-vpc-id=$VPC_ID --test-image-registry=$TEST_IMAGE_REGISTRY --ip-family=$IP_FAMILY || TEST_RESULT=fail
  else
    echo "Invalid IP_FAMILY input, choose from IPv4 or IPv6 only"
  fi
}

#Start the test
if [ "$IP_FAMILY" == "IPv6" ]; then
  enable_primary_ipv6_address
fi
run_ginkgo_test

# tail=-1 is added so that no logs are truncated
# https://github.com/kubernetes/kubectl/issues/812
echo "Fetch most recent aws-load-balancer-controller logs"
kubectl logs -l app.kubernetes.io/name=aws-load-balancer-controller --container $CONTAINER_NAME --tail=-1 -n kube-system || true

echo "Uncordon windows nodes"
toggle_windows_scheduling "uncordon"

echo "uninstalling aws load balancer controller"
if [[ $ADC_REGIONS == *"$REGION"* ]]; then
  kubectl delete -f $controller_yaml --timeout=60s || true
  kubectl delete -f  $cert_manager_yaml --timeout=60s || true
else
  helm uninstall aws-load-balancer-controller -n kube-system --timeout=60s || true
fi
echo "clean up resources from current run"
cleanUp

if [[ "$TEST_RESULT" == fail ]]; then
    echo "e2e tests failed."
    exit 1
fi

echo "Successfully finished the test suite $(($SECONDS / 60)) minutes and $(($SECONDS % 60)) seconds"
