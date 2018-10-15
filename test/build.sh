#!/usr/bin/env bash
set -e

if ! [[ "$0" =~ test/build.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

$(aws ecr get-login --no-include-email --region us-west-2)

if [[ -z "${GIT_COMMIT}" ]]; then
  GIT_COMMIT=$(git rev-parse --short=12 HEAD || echo "GitNotFound")
fi

_AWS_REGION=us-west-2
if [[ "${AWS_REGION}" ]]; then
  _AWS_REGION=${AWS_REGION}
fi

if [[ -z "${REGISTRY}" ]]; then
  REGISTRY=$(awstester ecr --region=${_AWS_REGION} get-registry)
fi

echo "Building:" ${REGISTRY}/alb:latest

# GO111MODULE=on go mod tidy -v
# GO111MODULE=on go mod vendor -v

CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) \
  go build -v \
  -o ./server \
  ./cmd

docker build \
  --tag ${REGISTRY}/alb:latest \
  --file ./Dockerfile .

docker push ${REGISTRY}/alb:latest
