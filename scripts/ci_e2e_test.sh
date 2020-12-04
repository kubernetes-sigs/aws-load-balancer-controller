#!/usr/bin/env bash

set -ueo pipefail

BUILD_ID=${BUILD_ID:-"$RANDOM"}
PULL_NUMBER=${PULL_NUMBER:-"0"}

echo $BUILD_ID
echo $PULL_NUMBER

aws sts get-caller-identity
export DOCKER_CLI_EXPERIMENTAL=enabled
docker buildx create --use
docker buildx ls
docker buildx build . --target bin --platform linux/amd64

