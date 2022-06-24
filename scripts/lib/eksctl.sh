#!/usr/bin/env bash

EKSCTL_DIR=${EKSCTL_DIR:="/tmp/lb-controller-e2e"}
EKSCTL_CLUSTER_DIR="${EKSCTL_DIR}/clusters"
EKSCTL_BINARY="${EKSCTL_DIR}/bin/eksctl"

EKSCTL_TEMPLATE_CLUSTER="$(dirname "${BASH_SOURCE[0]}")/eksctl_tmpl_cluster.yaml"
EKSCTL_TEMPLATE_IAM_SA="$(dirname "${BASH_SOURCE[0]}")/eksctl_tmpl_iam_sa.yaml"

#######################################
# Initialize EKSCTL package
#
# Globals:
#   EKSCTL_CLUSTER_DIR
#   EKSCTL_BINARY
# Arguments:
#   version         the version of eksctl
#
# sample: eksctl::init 0.100.0
#######################################
eksctl::init() {
  declare -r version="$1"

  local go_os="$(go env GOOS)"
  local os_arch="$(echo ${go_os:0:1} | tr [a-z] [A-Z])${go_os:1}_$(go env GOARCH)"
  local eksctl_download_url="https://github.com/weaveworks/eksctl/releases/download/${version}/eksctl_${os_arch}.tar.gz"
  local eksctl_binary_dir=$(dirname $EKSCTL_BINARY)

  if [[ ! -d ${EKSCTL_CLUSTER_DIR} ]]; then
    echo "creating EKSCTL clusters directory ${EKSCTL_CLUSTER_DIR}"

    if ! mkdir -p "${EKSCTL_CLUSTER_DIR}"; then
      echo "unable to create EKSCTL clusters directory ${EKSCTL_CLUSTER_DIR}" >&2
      return 1
    fi
  fi

  if [[ ! -d ${eksctl_binary_dir} ]]; then
    echo "creating EKSCTL binary directory ${eksctl_binary_dir}"

    if ! mkdir -p "${eksctl_binary_dir}"; then
      echo "unable to create EKSCTL binary directory ${eksctl_binary_dir}" >&2
      return 1
    fi
  fi

  if [[ ! -f ${EKSCTL_BINARY} ]]; then
    echo "downloading EKSCTL from ${eksctl_download_url} to ${EKSCTL_BINARY}"

    if ! curl --silent --location "${eksctl_download_url}" | tar xz -C "${eksctl_binary_dir}"; then
      echo "unable to download EKSCTL" >&2
      return 1
    fi

    if ! chmod u+x ${EKSCTL_BINARY}; then
      echo "unable to add execute permission for EKSCTL" >&2
      return 1
    fi
  fi

  return 0
}

#######################################
# Create k8s cluster.
#
# Globals:
#   EKSCTL_CLUSTER_DIR
#   EKSCTL_BINARY
#   EKSCTL_TEMPLATE_CLUSTER
# Arguments:
#   cluster_name        EKS cluster's name
#   region              aws region
#   k8s_version         EKS cluster's kubernetes version
#   instance_type       EKS cluster's instance type
#   node_count          EKS cluster's node count
#   cluster_kubeconfig  filename of cluster's kubeconfig
#
# sample: eksctl::create_cluster awesome-cluster us-west-2 1.19 m5.xlarge 4 awesome-cluster.kubeconfig
#######################################
eksctl::create_cluster() {
  declare -r cluster_name="$1" region="$2" k8s_version="$3" instance_type="$4" node_count="$5" cluster_kubeconfig="$6"

  local cluster_config="${EKSCTL_CLUSTER_DIR}/${cluster_name}.yaml"

  echo "creating cluster config into ${cluster_config}"
  cat "${EKSCTL_TEMPLATE_CLUSTER}" |
    yq eval ".metadata.name = \"${cluster_name}\"" - |
    yq eval ".metadata.region = \"${region}\"" - |
    yq eval ".metadata.version = \"${k8s_version}\"" - |
    yq eval ".nodeGroups[0].instanceType = \"${instance_type}\"" - |
    yq eval ".nodeGroups[0].desiredCapacity = ${node_count}" - >"${cluster_config}"

  cat "${cluster_config}"

  echo "creating cluster ${cluster_name}"
  if ! ${EKSCTL_BINARY} create cluster \
    -f "${cluster_config}" \
    --kubeconfig "${cluster_kubeconfig}"; then
    echo "unable to create cluster ${cluster_name}"
    return 1
  fi

  echo "created cluster ${cluster_name}"
  return 0
}

#######################################
# Delete k8s cluster.
#
# Globals:
#   EKSCTL_CLUSTER_DIR
#   EKSCTL_BINARY
# Arguments:
#   cluster_name        EKS cluster's name
#   region              aws region
#
# sample: eksctl::delete_cluster awesome-cluster us-west-2
#######################################
eksctl::delete_cluster() {
  declare -r cluster_name="$1" region="$2"

  local cluster_config="${EKSCTL_CLUSTER_DIR}/${cluster_name}.yaml"

  echo "deleting cluster ${cluster_name}"
  if ! ${EKSCTL_BINARY} delete cluster \
    -f "${cluster_config}" \
    --wait; then
    echo "unable to delete cluster ${cluster_name}"
    return 1
  fi

  echo "ueleted cluster ${cluster_name}"
  return 0
}

#######################################
# Get k8s cluster's VPC ID.
#
# Globals:
#   EKSCTL_BINARY
# Arguments:
#   cluster_name        EKS cluster's name
#   region              aws region
#
# sample: eksctl::get_cluster_vpc_id awesome-cluster us-west-2
#######################################
eksctl::get_cluster_vpc_id() {
  declare -r cluster_name="$1" region="$2"

  local cluster_info
  cluster_info=$(${EKSCTL_BINARY} get cluster --region "${region}" --name "${cluster_name}" --output yaml)
  if [[ $? -ne 0 ]]; then
    echo "unable to get cluster info" >&2
    return 1
  fi
  echo "${cluster_info}" | yq eval '.[0].ResourcesVpcConfig.VpcId' -
}

#######################################
# Create IAM role and Service Account
#
# Globals:
#   EKSCTL_BINARY
# Arguments:
#   cluster_name        EKS cluster's name
#   region              aws region
#   sa_namespace        namespace of service account
#   sa_name             name of service account
#   iam_policy_arn      arn of iam policy
#
# sample: eksctl::create_iamserviceaccount awesome-cluster us-west-2 awesome-ns awesome-name arn:aws:iam::xxxxx:policy/xxxxxx
#######################################
eksctl::create_iamserviceaccount() {
  declare -r cluster_name="$1" region="$2" sa_namespace="$3" sa_name="$4" iam_policy_arn="$5"

  echo "creating cluster SA ${sa_namespace}/${sa_name}"
  if ! ${EKSCTL_BINARY} create iamserviceaccount \
    --cluster "${cluster_name}" \
    --region "${region}" \
    --namespace "${sa_namespace}" \
    --name "${sa_name}" \
    --attach-policy-arn "${iam_policy_arn}" \
    --approve; then
    echo "unable to create cluster SA ${sa_namespace}/${sa_name}"
    return 1
  fi

  echo "created cluster SA ${sa_namespace}/${sa_name}"
  return 0
}
