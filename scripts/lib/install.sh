install_aws_lb_controller() {
    local vpc_id="${1}"
    local image="${2}"
    local create_service_account="${3}"
    local cluster_name="${CLUSTER_NAME}"
    local region="${REGION}"
    local optional_args=""

    if [[ "${create_service_account}" == "false" ]] ; then
        optional_args="--set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller"
    fi

    printf "Get update from eks-charts hub \n"
    helm repo add eks https://aws.github.io/eks-charts
    helm repo update

    printf "Install aws-load-balancer-controller Helm chart \n"
    helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller \
        -n kube-system \
        --set clusterName=$cluster_name --set region=$region --set vpcId=$vpc_id --set image.repository=$image $optional_args

    wait_until_deployment_ready aws-load-balancer-controller

}

wait_until_deployment_ready(){

    local deployment_name="${1}"
    local attempts=0
    local retries=60
    printf "Checking if deployment ${deployment_name} is ready \n"

    while [ ${attempts} -le ${retries} ]
    do
        desiredReplicas=$(kubectl get deployments.apps ${deployment_name} -n kube-system -ojsonpath="{.spec.replicas}")
        availableReplicas=$(kubectl get deployments.apps ${deployment_name} -n kube-system -ojsonpath="{.status.availableReplicas}")
        if [[ ! -z ${desiredReplicas} && ! -z ${availableReplicas} && "${desiredReplicas}" -eq "${availableReplicas}" ]]; then
            break
        fi
        attempts=$(( ${attempts} + 1 ))
        printf "attempt ${attempts}... \n"
        sleep 5
    done
    
    if [[ ${attempts} -gt ${retries} ]]; then
        printf "[Error] Deployment: ${deployment_name}, Replicas desired=${desiredReplicas} available=${availableReplicas}. Deployments failed to become ready \n"
        return 1
    else
        printf "Deployment: ${deployment_name}, Replicas desired=${desiredReplicas} available=${availableReplicas} \n"
        return 0
    fi
}

uninstall_aws_lb_controller(){

    if [[ "${PRINT_CONTROLLER_LOGS}" == "true" ]]; then
        printf "[INFO] Getting controller logs \n"
        kubectl logs -l app.kubernetes.io/name=aws-load-balancer-controller --container aws-load-balancer-controller --tail=-1 -n kube-system || true
    fi

    helm delete aws-load-balancer-controller -n kube-system --timeout=10m || printf "Uninstall AWS LB Helm release failed \n"
    kubectl delete serviceaccount aws-load-balancer-controller -n kube-system --timeout 10m || printf "Service account: 'aws-load-balancer-controller' Not Found \n"
}
