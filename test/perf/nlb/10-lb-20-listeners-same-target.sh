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
      targetPort: 80
      protocol: TCP
    - name: port3
      port: 892
      targetPort: 80
      protocol: TCP
    - name: port4
      port: 991
      targetPort: 80
      protocol: TCP
    - name: port5
      port: 1091
      targetPort: 80
      protocol: TCP
    - name: port6
      port: 8080
      targetPort: 80
      protocol: TCP
    - name: port7
      port: 8888
      targetPort: 80
      protocol: TCP
    - name: port8
      port: 889
      targetPort: 80
      protocol: TCP
    - name: port9
      port: 8889
      targetPort: 80
      protocol: TCP
    - name: port10
      port: 83
      targetPort: 80
      protocol: TCP
    - name: port11
      port: 84
      targetPort: 80
      protocol: TCP
    - name: port12
      port: 85
      targetPort: 80
      protocol: TCP
    - name: port13
      port: 86
      targetPort: 80
      protocol: TCP
    - name: port14
      port: 87
      targetPort: 80
      protocol: TCP
    - name: port15
      port: 88
      targetPort: 80
      protocol: TCP
    - name: port16
      port: 1231
      targetPort: 80
      protocol: TCP
    - name: port17
      port: 1232
      targetPort: 80
      protocol: TCP
    - name: port18
      port: 1233
      targetPort: 80
      protocol: TCP
    - name: port19
      port: 1234
      targetPort: 80
      protocol: TCP
    - name: port20
      port: 1235
      targetPort: 80
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

