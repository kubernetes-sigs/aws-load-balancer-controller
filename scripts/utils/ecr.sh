#!/usr/bin/env bash

set -ueo pipefail

AWS_REGION=${AWS_REGION:-"us-west-2"}


#######################################
# Generate docker image name for ecr
# Globals:
#   AWS_REGION
# Arguments:
#   img_repo
#   img_tag
#
# sample: ecr::name_image aws-alb-ingress-controlle v1.0.0 image_name
#######################################
ecr::name_image() {
    declare -r img_repo="$1" img_tag="$2"

    local aws_account_id=$(aws sts get-caller-identity --query Account --output text)
    if [[ -z "$aws_account_id" ]]; then
        echo "Unable to get AWS account ID" >&2
        return 1
    fi

    echo "$aws_account_id.dkr.ecr.$AWS_REGION.amazonaws.com/$img_repo:$img_tag"
    return 0
}

#######################################
# Push docker image to ECR
# Globals:
#   AWS_REGION
# Arguments:
#   img_name
#
# sample: ecr::push_image image_name
#######################################
ecr::push_image() {
    declare -r img_name="$1"
    eval $(aws ecr get-login --region $AWS_REGION --no-include-email)
    docker push "$img_name"
}