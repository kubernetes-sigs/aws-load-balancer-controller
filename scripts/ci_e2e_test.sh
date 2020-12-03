#!/usr/bin/env bash

set -ueo pipefail

BUILD_ID=${BUILD_ID:-"$RANDOM"}
PULL_NUMBER=${PULL_NUMBER:-"0"}

echo $BUILD_ID
echo $PULL_NUMBER

DOCKER_CLI_EXPERIMENTAL=enabled docker version
DOCKER_CLI_EXPERIMENTAL=enabled docker buildx ls
