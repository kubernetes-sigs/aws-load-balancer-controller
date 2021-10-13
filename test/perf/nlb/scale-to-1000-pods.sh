#!/bin/bash

. ./nlb-functions.sh


main() {
	echo_time "Starting Test"

	echo_time "Creating Deployment"
	deployment="perf-1000-scale"
	create_deployment $deployment 3

	wait_until_deployment_ready $deployment

	echo_time "Creating service"
	create_service $deployment 80 80


	get_lb_name "$deployment"-svc
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	check_target_group_health

	echo_time "Scaling deployment to 1000 replicas"
	kubectl scale deployment $deployment --replicas=1000
	wait_until_deployment_ready $deployment
	get_lb_name "$deployment"-svc
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	check_target_group_health
}

main $@

