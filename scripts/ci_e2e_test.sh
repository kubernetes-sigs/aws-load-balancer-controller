#!/usr/bin/env bash

set -ueo pipefail

# If running in PROW, BUILD_ID will be the prow build ID.
BUILD_ID=${BUILD_ID:-"$RANDOM"}
PULL_NUMBER=${PULL_NUMBER:-"0"}

AWS_REGION=${AWS_REGION:-"us-west-2"}

# Materials used to install ALB Ingress Controller.
RAW_CONTROLLER_POLICY_JSON="./docs/examples/iam-policy.json"
RAW_CONTROLLER_RBAC_YAML="./docs/examples/rbac-role.yaml"
RAW_CONTROLLER_DEPLOYMENT_YAML="./docs/examples/alb-ingress-controller.yaml"


# TODO(@M00nF1sh): rewrite whole script using python :(
# Globals to hold created resources.
CONTROLLER_POLICY_ARN=""
CONTROLLER_IMAGE_NAME=""


source $(dirname "${BASH_SOURCE}")/utils/cluster.sh
source $(dirname "${BASH_SOURCE}")/utils/ecr.sh

#######################################
# Install IAM Policy AWS ALB Ingress Controller into current cluster.
# Globals:
#   RAW_CONTROLLER_POLICY_JSON
#   CONTROLLER_POLICY_ARN
# Arguments:
#   cluster_name
#
#######################################
install_alb_ingress_policy() {
    declare -r cluster_name="$1"

    local policy_name="alb-ingress-controller-$cluster_name"
    echo "Creating iam policy $policy_name"
    CONTROLLER_POLICY_ARN=$(aws iam create-policy --region=$AWS_REGION --policy-name "$policy_name" --policy-document "file://$RAW_CONTROLLER_POLICY_JSON" --query "Policy.Arn" | sed 's/"//g')
    if [[ $? -ne 0 ]]; then
        echo "Unable to create iam policy $policy_name" >&2
        return 1
    fi

    local kops_node_role_name="nodes.${cluster_name}"
    echo "Attaching iam policy $CONTROLLER_POLICY_ARN to $kops_node_role_name"
    if ! aws iam attach-role-policy --region=$AWS_REGION --role-name=$kops_node_role_name --policy-arn=$CONTROLLER_POLICY_ARN; then
        echo "Unable to attach iam policy $CONTROLLER_POLICY_ARN to $kops_node_role_name" >&2
        return 1
    fi

    return 0
}

#######################################
# Uninstall IAM Policy AWS ALB Ingress Controller from current cluster.
# Globals:
#   None
# Arguments:
#   cluster_name
#   policy_arn
#
#######################################
uninstall_alb_ingress_policy() {
    declare -r cluster_name="$1" policy_arn="$2"

    if [[ ! -z "$policy_arn" ]]; then
        local kops_node_role_name="nodes.${cluster_name}"
        echo "Detaching iam policy $policy_arn from $kops_node_role_name"
        if ! aws iam detach-role-policy --region=$AWS_REGION --role-name=$kops_node_role_name --policy-arn=$policy_arn; then
            echo "Unable to detach iam policy $policy_arn from $kops_node_role_name" >&2
        fi

        echo "Deleting iam policy $policy_arn"
        if ! aws iam delete-policy --region=$AWS_REGION --policy-arn "$policy_arn"; then
            echo "Unable to delete iam policy $policy_arn" >&2
            return 1
        fi
    fi

    return 0
}

#######################################
# Install AWS ALB Ingress Controller into current cluster.
# Globals:
#   RAW_CONTROLLER_RBAC_YAML
#   RAW_CONTROLLER_DEPLOYMENT_YAML
# Arguments:
#   cluster_name
#   controller_image
#
#######################################
install_alb_ingress_controller() {
    declare -r cluster_name="$1" controller_image="$2"

    local controller_yaml="./controller.yaml"
    local controller_image_escaped=$(echo $controller_image | sed 's/\//\\\//g')
    if ! cat "$RAW_CONTROLLER_DEPLOYMENT_YAML" | \
        sed "s/# - --cluster-name=devCluster/- --cluster-name=$cluster_name/g" | \
        sed "s/image: docker.io\/amazon\/aws-alb-ingress-controller:.*/image: $controller_image_escaped/g" > "$controller_yaml"; then
        echo "Unable to init controller YAML for AWS ALB Ingress Controller" >&2
        return 1
    fi

    echo "Installing AWS ALB Ingress Controller"
    if ! cluster::apply $RAW_CONTROLLER_RBAC_YAML $controller_yaml; then
        echo "Unable to Installing AWS ALB Ingress Controller" >&2
        return 1
    fi

    return 0
}

#######################################
# Install AWS ALB Ingress Controller into current cluster.
# Globals:
#   PULL_NUMBER
# Arguments:
#   None
#
#######################################
build_push_controller_image() {
    CONTROLLER_IMAGE_NAME=$(ecr::name_image "aws-alb-ingress-controller" "pr-$PULL_NUMBER")
    if [[ $? -ne 0 ]]; then
        echo "Unable to name aws-alb-ingress-controller image" >&2
        return 1
    fi

    echo "Building aws-alb-ingress-controller image"
    if ! docker build -t "$CONTROLLER_IMAGE_NAME" ./; then
        echo "Unable to build aws-alb-ingress-controller image" >&2
        return 1
    fi

    echo "Pushing aws-alb-ingress-controller image as $CONTROLLER_IMAGE_NAME"
    if ! ecr::push_image "$CONTROLLER_IMAGE_NAME"; then
     echo "Unable to push aws-alb-ingress-controller image" >&2
        return 1
    fi

    return 0
}

setup() {
    declare -r cluster_name="$1"

    build_push_controller_image

    if ! cluster::init_KOPS 1.11.0; then
        echo "Unable to init kops" >&2
        exit 1
    fi

    if ! cluster::create $cluster_name 1.13.0 t3.medium 3; then
        echo "Unable to create cluster" >&2
        exit 1
    fi

    if ! install_alb_ingress_policy $cluster_name; then
        echo "Unable to install_alb_ingress_policy" >&2
        exit 1
    fi

    if ! install_alb_ingress_controller $cluster_name $CONTROLLER_IMAGE_NAME; then
        echo "Unable to install_alb_ingress_controller" >&2
        exit 1
    fi
}

cleanup() {
    declare -r cluster_name="$1"

    if ! uninstall_alb_ingress_policy $cluster_name $CONTROLLER_POLICY_ARN; then
        echo "Unable to uninstall_alb_ingress_policy" >&2
    fi

    if ! cluster::delete $cluster_name; then
        echo "Unable to delete cluster" >&2
    fi
}

test() {
    declare -r cluster_name="$1"
    vpc_id=$(aws ec2 describe-vpcs --region=$AWS_REGION --filters Name=tag-key,Values=kubernetes.io/cluster/$cluster_name --query 'Vpcs[0].VpcId' | sed 's/"//g')
    go get -u github.com/onsi/ginkgo/ginkgo
    $(go env GOBIN)/ginkgo -v test/e2e/  -- --kubeconfig=$HOME/.kube/config --cluster-name=$cluster_name --aws-region=$AWS_REGION --aws-vpc-id=$vpc_id
}

main() {
    local cluster_name="alb-ingress-e2e-${BUILD_ID}.k8s.local"

    trap "cleanup $cluster_name" EXIT
    setup $cluster_name
    test $cluster_name
}

main $@
