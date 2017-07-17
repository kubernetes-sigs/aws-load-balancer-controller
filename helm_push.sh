#!/bin/bash

set -e

/tmp/helm plugin install https://github.com/app-registry/appr-helm-plugin
/tmp/helm registry login -u ${QUAY_USERNAME} -p ${QUAY_PASSWORD} quay.io

cd alb-ingress-controller-helm
/tmp/helm registry push --namespace coreos quay.io

