#!/usr/bin/env bash

set -ueo pipefail
set -x

# shellcheck source=lib/iam.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/iam.sh"
# shellcheck source=lib/ecr.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/ecr.sh"
# shellcheck source=lib/eksctl.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/eksctl.sh"

# CI/CD environment
# If running in PROW, BUILD_ID will be the prow build ID.
BUILD_ID=${BUILD_ID:-"$RANDOM"}
PULL_NUMBER=${PULL_NUMBER:-"0"}
AWS_REGION=${AWS_DEFAULT_REGION:-"us-west-2"}

# Controller settings
LOCAL_GIT_VERSION=$(git describe --tags --always --dirty)
CONTROLLER_IMAGE_TAG=$LOCAL_GIT_VERSION
CONTROLLER_IMAGE_REPO="amazon/aws-load-balancer-controller"
CONTROLLER_IMAGE_NAME="" # will be fulfilled during build_push_controller_image

CONTROLLER_SA_NAMESPACE="kube-system"
CONTROLLER_SA_NAME="aws-load-balancer-controller"
CONTROLLER_IAM_POLICY_FILE="$(dirname "${BASH_SOURCE[0]}")/../docs/install/iam_policy.json"
CONTROLLER_IAM_POLICY_NAME="lb-controller-e2e-${PULL_NUMBER}-$BUILD_ID"
CONTROLLER_IAM_POLICY_ARN="" # will be fulfilled during setup_controller_iam_sa

# Cluster settings
EKSCTL_VERSION="0.34.0"
CLUSTER_NAME="lb-controller-e2e-${PULL_NUMBER}-$BUILD_ID"
CLUSTER_VERSION=${CLUSTER_VERSION:-"1.18"}
CLUSTER_INSTANCE_TYPE="m5.xlarge"
CLUSTER_NODE_COUNT="4"
CLUSTER_KUBECONFIG=${CLUSTER_KUBECONFIG:-"/tmp/lb-controller-e2e/clusters/${CLUSTER_NAME}.kubeconfig"}

#######################################
# Build and push ECR image for AWS Load Balancer Controller
#
# Globals:
#   AWS_REGION
#   CONTROLLER_IMAGE_REPO
#   CONTROLLER_IMAGE_TAG
#   CONTROLLER_IMAGE_NAME
# Arguments:
#   None
#######################################
build_push_controller_image() {
  if ! ecr::ensure_repository "${CONTROLLER_IMAGE_REPO}" "${AWS_REGION}"; then
    echo "unable to ensure ECR image repository" >&2
    return 1
  fi

  if ! ecr::login_repository "${CONTROLLER_IMAGE_REPO}" "${AWS_REGION}"; then
    echo "unable to login ECR image repository" >&2
    return 1
  fi

  if ecr::contains_image "${CONTROLLER_IMAGE_REPO}" "${CONTROLLER_IMAGE_TAG}" "${AWS_REGION}"; then
    echo "docker image ${CONTROLLER_IMAGE_REPO}:${CONTROLLER_IMAGE_TAG} already exists in ECR image repository. Skipping image build..."
    return 0
  fi

  CONTROLLER_IMAGE_NAME=$(ecr::name_image "${CONTROLLER_IMAGE_REPO}" "${CONTROLLER_IMAGE_TAG}" "${AWS_REGION}")
  if [[ $? -ne 0 ]]; then
    echo "unable to compute image name" >&2
    return 1
  fi

  echo "build and push docker image ${CONTROLLER_IMAGE_NAME}"
  DOCKER_CLI_EXPERIMENTAL=enabled docker buildx create --use
  DOCKER_CLI_EXPERIMENTAL=enabled docker buildx inspect --bootstrap

  # TODO: the first buildx build sometimes fails on new created builder instance.
  #  figure out why and remove this retry.
  n=0
  until [ "$n" -ge 2 ]; do
    DOCKER_CLI_EXPERIMENTAL=enabled docker buildx build . --target bin \
      --tag "${CONTROLLER_IMAGE_NAME}" \
      --push \
      --progress plain \
      --platform linux/amd64 && break
    n=$((n + 1))
    sleep 2
  done

  if [[ $? -ne 0 ]]; then
    echo "unable to build and push docker image" >&2
    return 1
  fi

  return 0
}

#######################################
# Setup test cluster
# Globals:
#   AWS_REGION
#   EKSCTL_VERSION
#   CLUSTER_NAME
#   CLUSTER_VERSION
#   CLUSTER_INSTANCE_TYPE
#   CLUSTER_NODE_COUNT
#   CLUSTER_KUBECONFIG
# Arguments:
#   None
#######################################
setup_cluster() {
  if ! eksctl::init "${EKSCTL_VERSION}"; then
    echo "unable to init eksctl" >&2
    return 1
  fi

  if ! eksctl::create_cluster "${CLUSTER_NAME}" "${AWS_REGION}" "${CLUSTER_VERSION}" "${CLUSTER_INSTANCE_TYPE}" "${CLUSTER_NODE_COUNT}" "${CLUSTER_KUBECONFIG}"; then
    echo "unable to create cluster ${CLUSTER_NAME}" >&2
    return 1
  fi

  return 0
}

#######################################
# Cleanup test cluster
# Globals:
#   AWS_REGION
#   CLUSTER_NAME
# Arguments:
#   None
#
#######################################
cleanup_cluster() {
  if ! eksctl::delete_cluster "${CLUSTER_NAME}" "${AWS_REGION}"; then
    echo "unable to delete cluster ${CLUSTER_NAME}" >&2
    return 1
  fi
}

#######################################
# Setup IAM role and Service Account for AWS Load Balancer Controller
#
# Globals:
#   AWS_REGION
#   CLUSTER_NAME
#   CONTROLLER_SA_NAMESPACE
#   CONTROLLER_SA_NAME
#   CONTROLLER_IAM_POLICY_NAME
#   CONTROLLER_IAM_POLICY_FILE
#   CONTROLLER_IAM_POLICY_ARN
# Arguments:
#   None
#######################################
setup_controller_iam_sa() {
  if [[ -z "${CONTROLLER_IAM_POLICY_ARN}" ]]; then
    echo "creating IAM policy for controller"

    CONTROLLER_IAM_POLICY_ARN=$(iam::create_policy "${CONTROLLER_IAM_POLICY_NAME}" "${CONTROLLER_IAM_POLICY_FILE}" "${AWS_REGION}")
    if [[ $? -ne 0 ]]; then
      echo "unable to create IAM policy for controller" >&2
      return 1
    fi

    echo "created IAM policy for controller: ${CONTROLLER_IAM_POLICY_ARN}"
  fi

  if ! eksctl::create_iamserviceaccount "${CLUSTER_NAME}" "${AWS_REGION}" "${CONTROLLER_SA_NAMESPACE}" "${CONTROLLER_SA_NAME}" "${CONTROLLER_IAM_POLICY_ARN}"; then
    echo "unable to create IAM role and service account for controller" >&2
    return 1
  fi

  return 0
}

#######################################
# Cleanup IAM role and Service Account for AWS Load Balancer Controller
#
# Globals:
#   AWS_REGION
#   CONTROLLER_IAM_POLICY_ARN
# Arguments:
#   None
#######################################
cleanup_controller_iam_sa() {
  if [[ -n "${CONTROLLER_IAM_POLICY_ARN}" ]]; then
    echo "deleting IAM policy for controller"

    if ! iam::delete_policy "${CONTROLLER_IAM_POLICY_ARN}" "${AWS_REGION}"; then
      echo "unable to delete IAM policy for controller" >&2
      return 1
    fi

    echo "deleted IAM policy for controller: ${CONTROLLER_IAM_POLICY_ARN}"
  fi
}

#######################################
# Test ECR image for AWS Load Balancer Controller
# Globals:
#   AWS_REGION
#   CLUSTER_NAME
#   CLUSTER_KUBECONFIG
#   CONTROLLER_IMAGE_NAME
#   CONTROLLER_SA_NAMESPACE
#   CONTROLLER_SA_NAME
# Arguments:
#   None
#######################################
test_controller_image() {
  local cluster_vpc_id
  cluster_vpc_id=$(eksctl::get_cluster_vpc_id "${CLUSTER_NAME}" "${AWS_REGION}")
  if [[ $? -ne 0 ]]; then
    echo "unable to get cluster vpc id" >&2
    return 1
  fi

  ginkgo -v -r test/e2e/ingress/ -- \
    --kubeconfig=${CLUSTER_KUBECONFIG} \
    --cluster-name=${CLUSTER_NAME} \
    --aws-region=${AWS_REGION} \
    --aws-vpc-id=${cluster_vpc_id} \
    --controller-image=${CONTROLLER_IMAGE_NAME}
}

#######################################
# Cleanup resources
# Globals:
#   None
# Arguments:
#   None
#######################################
cleanup() {
  sleep 60
  cleanup_cluster
  cleanup_controller_iam_sa
}

#######################################
# Entry point
# Globals:
#   IMAGE_REPO_APP_MESH_CONTROLLER
#   IMAGE_TAG
# Arguments:
#   None
#
#######################################
main() {
  go get github.com/mikefarah/yq/v4
  go get github.com/onsi/ginkgo/ginkgo

  build_push_controller_image

  trap "cleanup" EXIT
  setup_cluster
  setup_controller_iam_sa
  test_controller_image
}

main "$@"
