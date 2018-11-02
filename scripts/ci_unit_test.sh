COVER_PROFILE=profile.cov

report_coverage() {
    go get github.com/mattn/goveralls
    echo ${BUILD_ID}
    echo ${PULL_NUMBER}
    echo ${PROW_JOB_ID}
    echo yyyng

    BUILD_NUMBER=${BUILD_ID}  PULL_REQUEST_NUMBER=${PULL_NUMBER} $(go env GOBIN)/goveralls \
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