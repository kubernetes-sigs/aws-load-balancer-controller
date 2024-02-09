#!/usr/bin/env bash
set -euo pipefail

set +x

SCRIPTPATH="$( cd "$(dirname "$0")" ; pwd -P )"
TMP_DIR="$SCRIPTPATH/../../build"
PLATFORM=$(uname | tr '[:upper:]' '[:lower:]')
HELM3_VERSION="3.3.1"
HELM_DIR="${SCRIPTPATH}/../../helm"
LB_HELM_CHART=${HELM_DIR}/aws-load-balancer-controller

mkdir -p $TMP_DIR

if [ ! -x "$TMP_DIR/helm" ]; then
    echo " Downloading the \"helm3\" binary"
    curl -L https://get.helm.sh/helm-v$HELM3_VERSION-$PLATFORM-amd64.tar.gz | tar zxf - -C $TMP_DIR
    mv $TMP_DIR/$PLATFORM-amd64/helm $TMP_DIR/.
    chmod +x $TMP_DIR/helm
    echo " Downloaded the \"helm\" binary"
fi

export PATH=$TMP_DIR:$PATH

echo "=============================================================================="
echo "                     Linting Helm Chart w/ Helm v3"
echo "=============================================================================="
helm lint $LB_HELM_CHART

echo "=============================================================================="
echo "                   Generate Template w/ Helm v3"
echo "=============================================================================="

helm template aws-load-balancer-controller "${LB_HELM_CHART}" --debug --namespace=kube-system -f "${LB_HELM_CHART}/test.yaml" > /dev/null

