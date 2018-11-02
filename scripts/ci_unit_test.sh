COVER_PROFILE=profile.cov

report_coverage() {
    go get github.com/mattn/goveralls
    CI_NAME="prow" CI_BUILD_NUMBER=${BUILD_ID} CI_BRANCH=${PULL_BASE_REF} CI_PULL_REQUEST=${PULL_NUMBER} $(go env GOBIN)/goveralls \
           -coverprofile=$COVER_PROFILE \
           -service=prow \
           -repotoken $(cat /etc/coveralls-token/k8s-aws-alb-ingress-coveralls-token) \
           -ignore '*/*/mock*.go,*/*/*/mock*.go,*/*/*/*/mock*.go'
}

if [[ -z "${PROW_JOB_ID}" ]]; then
    go test ./...
else
    go test -covermode=count -coverprofile=$COVER_PROFILE ./...
    report_coverage
fi