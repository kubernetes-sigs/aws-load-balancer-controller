create_kops_cluster(){

    printf "Using ${KOPS_S3_BUCKET} as kops state store \n"
    if ! aws s3api head-bucket --bucket "${KOPS_S3_BUCKET}" 2>/dev/null; then
        printf "Creating bucket ${KOPS_S3_BUCKET} \n"
        aws s3api create-bucket --bucket "${KOPS_S3_BUCKET}" --region "${REGION}" --create-bucket-configuration LocationConstraint="${REGION}"
    fi

    if [ ! -f "${SSH_KEYS}" ]
    then
        printf "Creating SSH keys \n"
        ssh-keygen -t rsa -N '' -f "${SSH_KEYS}"
    else
        printf "SSH keys are already in place \n"
    fi

    echo "Installing kOps version: ${KOPS_BINARY_VERSION}"
    curl -LO https://github.com/kubernetes/kops/releases/download/"${KOPS_BINARY_VERSION}"/kops-linux-amd64

    chmod +x kops-linux-amd64
    mv kops-linux-amd64 "${KOPS_BIN}/kops"

    printf "Define ${CLUSTER_NAME} configuration \n"

    "${KOPS_BIN}/kops" create cluster \
        --cloud aws \
        --zones "${REGION}"a,"${REGION}"b \
        --networking amazonvpc \
        --container-runtime containerd \
        --node-count "${NODE_COUNT}" \
        --node-size "${NODE_SIZE}" \
        --ssh-public-key="${SSH_KEYS}.pub" \
        --kubernetes-version "${KUBERNETES_VERSION}" \
        --name "${CLUSTER_NAME}"

    printf "Starting cluster creation: ${CLUSTER_NAME} \n"
    "${KOPS_BIN}/kops" update cluster --name "${CLUSTER_NAME}" --yes --admin

    printf "Waiting for cluster to be up: ${CLUSTER_NAME} \n"
    "${KOPS_BIN}/kops" validate cluster --wait 15m

    printf "Cluster creation complete \n"
}

delete_kops_cluster(){

    "${KOPS_BIN}/kops" delete cluster --name "${CLUSTER_NAME}" --yes || (printf "Delete cluster failed ${CLUSTER_NAME} \n" && true)
    rm --recursive "${KOPS_BIN}" "${SSH_KEYS}"
}
