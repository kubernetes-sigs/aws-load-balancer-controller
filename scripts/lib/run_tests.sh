
run_test() {

  local extra_ginkgo_flags=$1
  local focus=$2
  local vpc_id=$3
  local failed=false

  printf "Starting the ginkgo tests from generated ginkgo test binaries with focus: $focus \n"
  if [ "${IP_FAMILY}" == "IPv4" ]; then 
    (CGO_ENABLED=0 GOOS="${OS_OVERRIDE}" ginkgo --no-color "${extra_ginkgo_flags}" --focus="${focus}" -v --timeout 60m --fail-on-pending "${GINKGO_TEST_BUILD}"/ingress.test -- --kubeconfig="${KUBECONFIG}" --cluster-name="${CLUSTER_NAME}" --aws-region="${REGION}" --aws-vpc-id="${vpc_id}" --ip-family="${IP_FAMILY}" || (printf "Ingress tests failed \n" && failed=true))
    (CGO_ENABLED=0 GOOS="${OS_OVERRIDE}" ginkgo --no-color "${extra_ginkgo_flags}" --focus="${focus}" -v --timeout 60m --fail-on-pending "${GINKGO_TEST_BUILD}"/service.test -- --kubeconfig="${KUBECONFIG}" --cluster-name="${CLUSTER_NAME}" --aws-region="${REGION}" --aws-vpc-id="${vpc_id}" --ip-family="${IP_FAMILY}" || (printf "Service tests failed \n" && failed=true))
  elif [ "${IP_FAMILY}" == "IPv6" ]; then
    (CGO_ENABLED=0 GOOS="${OS_OVERRIDE}" ginkgo --no-color "${extra_ginkgo_flags}" --focus="${focus}" --skip="instance" -v --timeout 60m --fail-on-pending "${GINKGO_TEST_BUILD}"/ingress.test -- --kubeconfig="${KUBECONFIG}" --cluster-name="${CLUSTER_NAME}" --aws-region="${REGION}" --aws-vpc-id="${vpc_id}" --ip-family="${IP_FAMILY}" || (printf "Ingress tests failed" && failed=true))
    (CGO_ENABLED=0 GOOS="${OS_OVERRIDE}" ginkgo --no-color "${extra_ginkgo_flags}" --focus="${focus}" --skip="instance" -v --timeout 60m --fail-on-pending "${GINKGO_TEST_BUILD}"/service.test -- --kubeconfig="${KUBECONFIG}" --cluster-name="${CLUSTER_NAME}" --aws-region="${REGION}" --aws-vpc-id="${vpc_id}" --ip-family="${IP_FAMILY}" || (printf "Service tests failed" && failed=true))
  else
    printf "[Error] Invalid IP_FAMILY input, choose from IPv4 or IPv6 only \n"
    return 1
  fi

  if [[ $failed == true ]]; then
    printf "[Error] Failed ginkgo tests: $focus \n"
    return 1
  else
    return 0
  fi
}
