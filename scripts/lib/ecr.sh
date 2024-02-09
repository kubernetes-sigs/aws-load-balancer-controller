#!/usr/bin/env bash

#######################################
# Create ECR repository if not exists
#
# Globals:
#   None
# Arguments:
#   img_repo      name of ECR repository
#   region        aws region
#
# sample: ecr::ensure_repository awesome-repo us-west-2
ecr::ensure_repository() {
  declare -r img_repo="$1" region="$2"

  if ! aws ecr describe-repositories \
    --region "${region}" \
    --repository-names "${img_repo}" >/dev/null 2>&1; then
    echo "creating ECR repository: ${img_repo}"
    aws ecr create-repository --region "${region}" --repository-name "${img_repo}"
  fi
}

#######################################
# Login into ECR repository
#
# Globals:
#   None
# Arguments:
#   img_repo      name of ECR repository
#   region        aws region
#
# sample: ecr::login_repository awesome-repo us-west-2
#######################################
ecr::login_repository() {
  declare -r img_repo="$1" region="$2"

  local repo_uri
  repo_uri=$(ecr::get_repository_uri "${img_repo}" "${region}")
  if [[ $? -ne 0 ]]; then
    echo "unable to obtain ECR repository URI" >&2
    return 1
  fi

  aws ecr get-login-password \
    --region "${region}" | docker login \
    --username AWS \
    --password-stdin "${repo_uri}"
}

#######################################
# Generate docker image name for ecr
#
# Globals:
#   None
# Arguments:
#   img_repo      name of ECR repository
#   img_tag       tag of image
#   region        aws region
#
# sample: ecr::name_image awesome-repo v1.0.0 us-west-2
#######################################
ecr::name_image() {
  declare -r img_repo="$1" img_tag="$2" region="$3"

  local repo_uri
  repo_uri=$(ecr::get_repository_uri "${img_repo}" "${region}")
  if [[ $? -ne 0 ]]; then
    echo "unable to obtain ECR repository URI" >&2
    return 1
  fi

  echo "${repo_uri}:${img_tag}"
}

#######################################
# Check whether docker image exists in ECR
#
# Globals:
#   None
# Arguments:
#   img_repo      name of ECR repository
#   img_tag       tag of image
#   region        aws region
#
# sample: ecr::contains_image awesome-repo v1.0.0 us-west-2
#######################################
ecr::contains_image() {
  declare -r img_repo="$1" img_tag="$2" region="$3"

  if ! aws ecr describe-images \
    --region "${region}" \
    --repository-name "${img_repo}" \
    --image-ids imageTag="${img_tag}" >/dev/null 2>&1; then
    return 1
  fi

  return 0
}

#######################################
# Get URI of ECR repository
#
# Globals:
#   None
# Arguments:
#   img_repo      name of ECR repository
#   region        aws region
#
# sample: ecr::get_repository_uri awesome-repo us-west-2
ecr::get_repository_uri() {
  declare -r img_repo="$1" region="$2"

  aws ecr describe-repositories \
    --region "${region}" \
    --repository-names "${img_repo}" \
    --query 'repositories[0].repositoryUri' \
    --output=text 2>/dev/null
}
