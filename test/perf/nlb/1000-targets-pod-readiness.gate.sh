#!/bin/bash

. ./nlb-functions.sh


main() {
	echo_time "Starting Test"
	ns=readiness
	deployment="perf-1000-scale-pr"

	kubectl create namespace $ns
	kubectl label namespaces $ns elbv2.k8s.aws/pod-readiness-gate-inject=enabled

	sleep 2
	echo_time "Creating service"
	create_service $deployment 80 80 $ns

	# Wait for the service to get reconciled
	sleep 2

	echo_time "Creating Deployment"
	create_deployment $deployment 1 $ns

	wait_until_deployment_ready $deployment $ns

	get_lb_name "$deployment"-svc $ns
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	#check_target_group_health

	echo_time "Scaling deployment to 1000 replicas"
	kubectl scale deployment $deployment -n $ns --replicas=1000
	#echo_time "Scaling deployment to 3 replicas"
	#kubectl scale deployment $deployment -n $ns --replicas=3
	wait_until_deployment_ready $deployment $ns
	get_lb_name "$deployment"-svc $ns
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	check_target_group_health 1000
	kubectl delete namespace readiness
}

main $@

