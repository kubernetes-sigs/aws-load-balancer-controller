#!/usr/bin/env bash

#######################################
# Create IAM policy
#
# Globals:
#   None
# Arguments:
#   policy_name     name of IAM policy
#   policy_file     file of IAM policy
#   region          aws region
#
# sample: iam::create_policy awesome-repo path/to/awesome-repo.json us-west-2
iam::create_policy() {
  declare -r policy_name="$1" policy_file="$2" region="$3"
  aws iam create-policy \
    --region "${region}" \
    --policy-name "${policy_name}" \
    --policy-document "file://${policy_file}" \
    --query 'Policy.Arn' --output text 2>/dev/null
}

#######################################
# Delete IAM policy
#
# Globals:
#   None
# Arguments:
#   policy_arn      arn of IAM policy
#   region          aws region
#
# sample: iam::delete_policy arn:aws:iam::xxxxx:policy/xxxxxx
iam::delete_policy() {
  declare -r policy_arn="$1" region="$2"

  aws iam delete-policy \
    --region "${region}" \
    --policy-arn "${policy_arn}"
}
