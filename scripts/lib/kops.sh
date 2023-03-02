kops::create_kops_cluster(){
    set -x
    echo "Using ${KOPS_S3_BUCKET} as kops state store"
    if ! aws s3api head-bucket --bucket "${KOPS_S3_BUCKET}" 2>/dev/null; then
        echo "Creating bucket ${KOPS_S3_BUCKET}"
        aws s3api create-bucket --bucket "${KOPS_S3_BUCKET}" --region "${REGION}" --create-bucket-configuration LocationConstraint="${REGION}"
    fi

    if [ ! -f ${SSH_KEYS} ]
    then
        echo "Creating SSH keys"
        ssh-keygen -t rsa -N '' -f "${SSH_KEYS}"
    else
        echo "SSH keys are already in place"
    fi

    echo "Installing kOps version: ${KOPS_BINARY_VERSION}"
    curl -LO https://github.com/kubernetes/kops/releases/download/${KOPS_BINARY_VERSION}/kops-linux-amd64
    chmod +x kops-linux-amd64
    mkdir -p $KOPS_BIN
    mv kops-linux-amd64 $KOPS_BIN/kops

    echo "Define ${CLUSTER_NAME} configuration"

    $KOPS_BIN/kops create cluster \
        --cloud aws \
        --zones "${REGION}"a,"${REGION}"b \
        --networking amazonvpc \
        --container-runtime containerd \
        --node-count "${NODE_COUNT}" \
        --node-size "${NODE_SIZE}" \
        --ssh-public-key="${SSH_KEYS}.pub" \
        --kubernetes-version "${KUBERNETES_VERSION}" \
        --name "${CLUSTER_NAME}"
    echo "Starting cluster creation: ${CLUSTER_NAME}"
    $KOPS_BIN/kops update cluster --name "${CLUSTER_NAME}" --yes --admin

    echo "Waiting for cluster to be up: ${CLUSTER_NAME}"
    $KOPS_BIN/kops validate cluster --wait 15m

    echo "Cluster creation complete"
}

kops::delete_kops_cluster(){

    echo "Deleting cluster..."
    $KOPS_BIN/kops delete cluster --name "${CLUSTER_NAME}" --yes || (echo "Delete cluster failed ${CLUSTER_NAME}" && true)
    rm --recursive "${KOPS_BIN}" "${SSH_KEYS}"
}
