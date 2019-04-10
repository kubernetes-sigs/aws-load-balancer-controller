#!/usr/bin/env bash
set -e

COVER_PROFILE=profile.cov

report_coverage() {
    go install -v github.com/mattn/goveralls
    # need to override prow's BUILD_NUMBER to "" so it won't be reported as jobID to avoid 5xx error :D
    BUILD_NUMBER="" PULL_REQUEST_NUMBER=${PULL_NUMBER} $GOBIN/goveralls \
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