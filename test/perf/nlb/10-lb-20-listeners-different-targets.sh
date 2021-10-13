#!/bin/bash

. ./nlb-functions.sh

apply_svc() {
cat <<EOF | kubectl apply -f -
kind: Service
apiVersion: v1
metadata:
  name: $1-svc
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb-ip"
spec:
  ports:
    - name: port1
      port: 80
      targetPort: 80
      protocol: TCP
    - name: port2
      port: 891
      targetPort: 81
      protocol: TCP
    - name: port3
      port: 892
      targetPort: 82
      protocol: TCP
    - name: port4
      port: 991
      targetPort: 83
      protocol: TCP
    - name: port5
      port: 1091
      targetPort: 84
      protocol: TCP
    - name: port6
      port: 8080
      targetPort: 85
      protocol: TCP
    - name: port7
      port: 8888
      targetPort: 86
      protocol: TCP
    - name: port8
      port: 889
      targetPort: 87
      protocol: TCP
    - name: port9
      port: 8889
      targetPort: 88
      protocol: TCP
    - name: port10
      port: 83
      targetPort: 89
      protocol: TCP
  type: NodePort
  selector:
    app: $1
EOF
}


main() {
	echo_time "Starting Test, create 30 load balancers"
	limit=30
	deployment="multtlb-targetsame-"
	for i in $(seq 1 $limit); do
		dep="$deployment"$i
		echo_time "Creating Deployment $dep"
		create_deployment $dep 10
	
		#wait_until_deployment_ready $dep

		echo_time "Creating service"
		apply_svc $dep
	done

	get_lb_name "$deployment"1-svc
	echo_time "Deployed loadbalancer $lbName"
	query_lb_arn $lbName

	get_target_group_arn $lbArn
	echo_time "TargetGroup ARN $tgtGrpArn"

	check_target_group_health

	for i in $(seq 1 $limit); do
		svc="$deployment$i"-svc
		prov=$(kubectl get svc $svc -ojsonpath='{.status.loadBalancer.ingress[0].hostname}' |awk -F- {'print $1"-"$2"-"$3"-"$4'})
		echo_time "Providioned lb $prov for svc $svc"
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

