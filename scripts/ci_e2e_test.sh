#!/usr/bin/env bash

set -ueo pipefail

BUILD_ID=${BUILD_ID:-"$RANDOM"}
PULL_NUMBER=${PULL_NUMBER:-"0"}

echo $BUILD_ID
echo $PULL_NUMBER

export DOCKER_CLI_EXPERIMENTAL=enabled
docker version
docker info
#docker pull docker.io/docker/dockerfile:experimental
#docker pull docker.io/library/golang:1.15.0
#docker pull docker.io/library/amazonlinux:2
docker buildx ls
docker buildx build . --target bin --platform linux/amd64,linux/arm64
docker buildx create --use