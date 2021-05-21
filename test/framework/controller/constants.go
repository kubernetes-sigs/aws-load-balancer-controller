package controller

import "time"

const (
	EKSHelmChartsRepo                            = "https://aws.github.io/eks-charts"
	AWSLoadBalancerControllerHelmChart           = "aws-load-balancer-controller"
	AWSLoadBalancerControllerHelmRelease         = "aws-load-balancer-controller"
	AWSLoadBalancerControllerInstallationTimeout = 2 * time.Minute
)
