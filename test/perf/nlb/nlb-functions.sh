#!/bin/bash
#set -o errexit
#set -o nounset
#set -o pipefail

lbName=""
lbArn=""
tgtGrpArn=""
tgHealth=""

echo_time() {
    date +"%D %T $*"
}

create_deployment() {
NS=""
if [ ! -z $3 ]; then
	NS="-n $3"
fi
cat <<EOF | kubectl apply $NS -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: $1
spec:
  strategy:
    rollingUpdate:
      maxSurge: 100%
      maxUnavailable: 25%
    type: RollingUpdate
  replicas: $2
  selector:
    matchLabels:
      app: $1
  template:
    metadata:
      labels:
        app: $1
        ports: multi
    spec:
      containers:
        - name: multi
          imagePullPolicy: Always
          image: "kishorj/hello-multi:v1"
          ports:
            - name: http
              containerPort: 80
EOF
}

create_service() {
NS=""
if [ ! -z $4 ]; then
	NS="-n $4"
fi
echo $1
cat <<EOF | kubectl apply $NS -f -
kind: Service
apiVersion: v1
metadata:
  name: $1-svc
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb-ip"
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"
spec:
  ports:
    - name: port
      port: $2
      targetPort: $3
      protocol: TCP
  type: NodePort
  selector:
    app: $1
EOF
echo $1
}


wait_until_deployment_ready() {
	NS=""
	if [ ! -z $2 ]; then
		NS="-n $2"
	fi
	echo_time "Checking if deployment $1 is ready"
	for i in $(seq 1 60); do
		desiredReplicas=$(kubectl get deployments.apps $1 $NS -ojsonpath="{.spec.replicas}")
		availableReplicas=$(kubectl get deployments.apps $1 $NS -ojsonpath="{.status.availableReplicas}")
		if [[ ! -z $desiredReplicas && ! -z $availableReplicas && "$desiredReplicas" -eq "$availableReplicas" ]]; then
			break
		fi
		echo -n "."
		sleep 2
	done
	echo_time "Deployment $1 $NS replicas desired=$desiredReplicas available=$availableReplicas"
}

get_lb_name() {
	NS=""
	if [ ! -z $2 ]; then
		NS="-n $2"
	fi
	echo_time "Looking up service $1 $NS"
	for i in $(seq 1 10); do
		lbName=$(kubectl get svc $1 $NS -ojsonpath='{.status.loadBalancer.ingress[0].hostname}' |awk -F- {'print $1"-"$2"-"$3"-"$4'})
		if [ "$lbName" != "" ]; then
			break
		fi
		sleep 2
		echo -n "."
	done
	echo
	echo_time "$lbName"

}

delete_deployment() {
	kubectl delete deployment $1
}

query_lb_arn() {
	lbArn=$(aws elbv2 describe-load-balancers --names $1 | jq -j '.LoadBalancers | .[] | .LoadBalancerArn')
	echo_time "LoadBalancer ARN $lbArn"
}

get_target_group_arn() {
	tgtGrpArn=$(aws elbv2 describe-listeners --load-balancer-arn $lbArn | jq -j ".Listeners[0].DefaultActions[0].TargetGroupArn")
}

get_target_group_health() {
	tgHealth=$(aws elbv2 describe-target-health --target-group-arn $tgtGrpArn | jq -r '.TargetHealthDescriptions[] | [.TargetHealth.State][]')
}

check_target_group_health() {
	echo_time "Checking target group health "
	numreplicas=$1
	lastcount=0
	for i in  $(seq 1 60); do 
		count=0
		echo -n "."
		get_target_group_health
		for status in $tgHealth; do
			let "count+=1"
		done
		if [ $count -ne $lastcount ]; then
			lastcount=$count
			echo_time "Got $count targets"
		fi

		something_else=0
		for status in $tgHealth; do
			if [[ "$status" != "healthy" && "$status" != "unhealthy" ]];then
				something_else=1
				break
			fi
		done
		if [ $something_else -eq 0 ]; then
			if [ -z $numreplicas ]; then
				break
			fi
			if [ "$numreplicas" -eq "$count" ]; then
				break
			fi
		fi
		sleep 10
	done
	echo
	echo_time "Got $lastcount targets"
	echo_time "Target health"
	echo $tgHealth
}
