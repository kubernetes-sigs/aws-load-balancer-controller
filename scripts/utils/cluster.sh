#!/usr/bin/env bash

set -ueo pipefail

AWS_REGION=${AWS_REGION:-"us-west-2"}
AWS_AVAILABILITY_ZONES=${AWS_AVAILABILITY_ZONES:-"us-west-2a,us-west-2b,us-west-2c"}
SSH_PUBLIC_KEY=${AWS_SSH_PUBLIC_KEY_FILE:-"~/.ssh/id_rsa.pub"}

KOPS_BINARY=${KOPS_BINARY:-"/tmp/alb-e2e/kops"}
KOPS_STATE_BUCKET=${KOPS_STATE_BUCKET:-"aws-alb-ingress-controller-607362164682"}

#######################################
# Initialize cluster package
# Globals:
#   AWS_REGION
#   KOPS_BINARY
#   KOPS_STATE_BUCKET
# Arguments:
#   kops_version
#
# sample: cluster::init_KOPS 1.11.0
#######################################
cluster::init_KOPS() {
    declare -r kops_version="$1"

    local kops_arch="kops-$(go env GOOS)-amd64"
    local kops_url="https://github.com/kubernetes/kops/releases/download/$kops_version/$kops_arch"
    local kops_binary_dir=$(dirname $KOPS_BINARY)

    if [[ ! -d $kops_binary_dir ]]; then
        echo "Creating KOPS directory $kops_binary_dir"

        if ! mkdir -p "$kops_binary_dir"; then
            echo "Unable to create KOPS directory $kops_binary_dir" >&2
            return 1
        fi
    fi

    if [[ ! -f $KOPS_BINARY ]]; then
        echo "Downloading KOPS from $kops_url to $KOPS_BINARY"

        if ! curl -L -X GET $kops_url -o $KOPS_BINARY; then
            echo "Unable to download KOPS binary" >&2
            return 1
        fi
        if ! chmod u+x $KOPS_BINARY; then
            echo "Unable to add execute permission for KOPS binary" >&2
            return 1
        fi
    fi

    if ! aws s3api head-bucket --bucket "$KOPS_STATE_BUCKET" 2>/dev/null; then
        echo "Creating S3 bucket $KOPS_STATE_BUCKET as KOPS state store"
        if ! aws s3 mb "s3://$KOPS_STATE_BUCKET" --region "$AWS_REGION" 1>/dev/null; then
            echo "Unable to create S3 bucket $KOPS_STATE_BUCKET" >&2
            return 1
        fi
    fi

    return 0
}

#######################################
# Create k8s cluster.
# Globals:
#   AWS_AVAILABILITY_ZONES
#   KOPS_BINARY
#   KOPS_STATE_BUCKET
#   SSH_PUBLIC_KEY
# Arguments:
#   cluster_name
#   k8s_version
#   node_size
#   node_count
#
# sample: cluster::create alb-e2e.k8s.local 1.13.0 t3.medium 3
#######################################
cluster::create() {
    declare -r cluster_name="$1" k8s_version="$2" node_size="$3" node_count="$4"

    echo "Creating cluster $cluster_name"
    if ! $KOPS_BINARY create cluster \
            --name  "$cluster_name" \
            --state "s3://$KOPS_STATE_BUCKET" \
            --kubernetes-version="$k8s_version" \
            --networking amazon-vpc-routed-eni \
            --zones "$AWS_AVAILABILITY_ZONES" \
            --node-size "$node_size" \
            --node-count "$node_count" \
            --ssh-public-key="$SSH_PUBLIC_KEY" \
            --yes; then
        echo "Unable to create cluster $cluster_name"
        return 1
    fi

    echo "Waiting for cluster $cluster_name to be ready"
    while true
    do
        if ! $KOPS_BINARY validate cluster \
                --name  "$cluster_name" \
                --state "s3://$KOPS_STATE_BUCKET"; then
            sleep 30
        else
            break
        fi
    done

    return 0
}

#######################################
# Delete k8s cluster.
# Globals:
#   KOPS_BINARY
#   KOPS_STATE_BUCKET
# Arguments:
#    cluster_name
#
# sample: cluster::delete alb_ingress_e2e.k8s.local
#######################################
cluster::delete() {
    declare -r cluster_name="$1"

    echo "Deleting cluster $cluster_name"
    if ! $KOPS_BINARY delete cluster \
           --name  "$cluster_name" \
           --state "s3://$KOPS_STATE_BUCKET" \
           --yes; then
        echo "Unable to delete cluster $cluster_name"
    fi
}

#######################################
# Apply k8s YAMLs into current cluster.
# Globals:
#   None
# Arguments:
#   list of YAMLs
#
# sample: install_alb_ingress_controller
#######################################
cluster::apply() {
    for yaml in "$@"
    do
        echo "Applying $yaml"
        if ! kubectl apply -f "$yaml"; then
            echo "Unable to apply $yaml" >&2
            return 1
        fi
    done

    return 0
}