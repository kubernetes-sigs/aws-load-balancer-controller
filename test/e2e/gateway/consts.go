package gateway

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
)
