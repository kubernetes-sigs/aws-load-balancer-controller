#!/bin/bash

. ./nlb-functions.sh


main() {
	if [ "$#" -ne 1 ]; then
    		echo "Usage $0 traffic.out"
		return
	fi

	echo_time "Starting Test"
	ns=traffic-test
	deployment=traffic-srv

	echo_time "Creating service"
	create_service $deployment 80 80 $ns

	echo_time "Creating Deployment"
	create_deployment $deployment 100 $ns

	wait_until_deployment_ready $deployment $ns

	get_lb_name "$deployment"-svc $ns
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	check_target_group_health

	lbDNSName=$(kubectl get svc "$deployment"-svc -n $ns -ojsonpath='{.status.loadBalancer.ingress[0].hostname}')
	for i in $(seq 1 1000); do
		curl http://$lbDNSName/ 2>/dev/null | grep Hostname | awk {'print $2'} >> $1
	done
}

main $@

