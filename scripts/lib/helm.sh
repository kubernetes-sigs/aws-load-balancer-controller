helm::install_aws_lb_controller() {
    local vpc_id="${1}"
    local image="${2}"
    local create_service_account="${3}"
    local cluster_name="${CLUSTER_NAME}"
    local region="${REGION}"
    local optional_args=""

    if [[ "${create_service_account}" == "false" ]] ; then
        optional_args="--set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller"
    fi

    echo "Get update from eks-charts hub"
    helm repo add eks https://aws.github.io/eks-charts
    helm repo update

    echo "Install aws-load-balancer-controller Helm chart"
    helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller \
        -n kube-system \
        --set clusterName=$cluster_name --set region=$region --set vpcId=$vpc_id --set image.repository=$image $optional_args

    helm::wait_until_deployment_ready aws-load-balancer-controller

}

helm::wait_until_deployment_ready(){

    local deployment_name="${1}"
    local attempts=0
    local retries=60
    echo "Checking if deployment ${deployment_name} is ready"

    while [ ${attempts} -le ${retries} ]
    do
        desiredReplicas=$(kubectl get deployments.apps ${deployment_name} -n kube-system -ojsonpath="{.spec.replicas}")
        availableReplicas=$(kubectl get deployments.apps ${deployment_name} -n kube-system -ojsonpath="{.status.availableReplicas}")
        if [[ ! -z ${desiredReplicas} && ! -z ${availableReplicas} && "${desiredReplicas}" -eq "${availableReplicas}" ]]; then
            break
        fi
        attempts=$(( ${attempts} + 1 ))
        echo "attempt ${attempts}..."
        sleep 5
    done
    
    if [[ ${attempts} -gt ${retries} ]]; then
        echo "[Error] Deployment: ${deployment_name}, Replicas desired=${desiredReplicas} available=${availableReplicas}. Deployments failed to become ready"
        return 1
    else
        echo "Deployment: ${deployment_name}, Replicas desired=${desiredReplicas} available=${availableReplicas}"
        return 0
    fi
}

helm::uninstall_aws_lb_controller(){

    if [[ "${PRINT_CONTROLLER_LOGS}" == "true" ]]; then
        echo "[INFO] Getting controller logs"
        kubectl logs -l app.kubernetes.io/name=aws-load-balancer-controller --container aws-load-balancer-controller --tail=-1 -n kube-system || true
    fi

    helm delete aws-load-balancer-controller -n kube-system --timeout=10m || echo "Uninstall AWS LB Helm release failed \n"
    kubectl delete serviceaccount aws-load-balancer-controller -n kube-system --timeout 10m || echo "Service account: 'aws-load-balancer-controller' Not Found"
}
