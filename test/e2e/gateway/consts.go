package gateway

import "sigs.k8s.io/aws-load-balancer-controller/test/framework/verifier"

const (
	appContainerPort        = 80
	udpContainerPort        = 8080
	defaultNumReplicas      = 3
	defaultName             = "gateway-e2e"
	udpDefaultName          = defaultName + "-udp"
	defaultGatewayClassName = "gwclass-e2e"
	defaultLbConfigName     = "lbconfig-e2e"
	defaultTgConfigName     = "tgconfig-e2e"
	udpDefaultTgConfigName  = defaultTgConfigName + "-udp"
	testHostname            = "*.elb.us-west-2.amazonaws.com"
)

// Common settings for ALB target group health checks
var DEFAULT_ALB_TARGET_GROUP_HC = &verifier.TargetGroupHC{
	Protocol:           "HTTP",
	Port:               "traffic-port",
	Path:               "/",
	Interval:           15,
	Timeout:            5,
	HealthyThreshold:   3,
	UnhealthyThreshold: 3,
}
