#!/bin/bash

. ./nlb-functions.sh


main() {
	echo_time "Starting Test"

	echo_time "Creating Deployment"
	create_deployment "perf-1000" 1000

	wait_until_deployment_ready "perf-1000"

	echo_time "Creating service"
	create_service "perf-1000" 80 80


	get_lb_name "perf-1000-svc"
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	check_target_group_health
}

main $@

