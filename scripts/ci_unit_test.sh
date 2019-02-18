#!/usr/bin/env bash
set -e

COVER_PROFILE=profile.cov

report_coverage() {
    go get github.com/mattn/goveralls
    CIRCLE_BUILD_NUM=${BUILD_ID}  CIRCLE_PR_NUMBER=${PULL_NUMBER} $(go env GOBIN)/goveralls \
           -coverprofile=$COVER_PROFILE \
           -service=prow \
           -repotoken $(cat /etc/coveralls-token/k8s-aws-alb-ingress-coveralls-token) \
           -ignore '*/*/mock*.go,*/*/*/mock*.go,*/*/*/*/mock*.go'
}

if [[ -z "${PROW_JOB_ID}" ]]; then
    go test ./internal/...
else
    go test -covermode=count -coverprofile=$COVER_PROFILE ./internal/...
    report_coverage
fi