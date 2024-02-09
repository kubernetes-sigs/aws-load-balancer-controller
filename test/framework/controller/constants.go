package controller

import "time"

const (
	AWSLoadBalancerControllerHelmChart           = "aws-load-balancer-controller"
	AWSLoadBalancerControllerHelmRelease         = "aws-load-balancer-controller"
	AWSLoadBalancerControllerInstallationTimeout = 2 * time.Minute
)
