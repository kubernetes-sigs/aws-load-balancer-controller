#!/bin/bash

. ./nlb-functions.sh


main() {
	echo_time "Starting Test, create 30 load balancers"
	limit=30
	deployment="thirtylb-perf-"
	for i in $(seq 1 $limit); do
		dep="$deployment"$i
		echo_time "Creating Deployment $dep"
		create_deployment $dep 10
	
		echo_time "Creating service"
		create_service $dep 80 80

	done


	get_lb_name "$deployment"1-svc
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	check_target_group_health

	for i in $(seq 1 $limit); do
		dep="$deployment"$i
		svc="$deployment$i"-svc
		prov=$(kubectl get svc $svc -ojsonpath='{.status.loadBalancer.ingress[0].hostname}' |awk -F- {'print $1"-"$2"-"$3"-"$4'})
		echo_time "Providioned lb $prov for svc $svc"
		get_lb_name $svc

		echo_time "Deployed loadbalancer $lbName"
		query_lb_arn $lbName

		get_target_group_arn $lbArn
		echo_time "TargetGroup ARN $tgtGrpArn"

		check_target_group_health
	done

	echo "Press any key to proceed with resource deletion"
	read anykey

	for i in $(seq 1 $limit); do
		dep="$deployment"$i
		svc="$deployment$i"-svc
		echo_time "deleting $dep and $svc"
		kubectl delete deploy $dep
		kubectl delete svc $svc
	done

}

main $@

