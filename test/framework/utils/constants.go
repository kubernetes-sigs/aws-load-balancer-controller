package utils

// Test image paths; vars so downstream can override with resolved (non-`:latest`) tags.
var (
	HelloImage       = "networking-e2e-test-images/hello-multi:latest"
	ColortellerImage = "networking-e2e-test-images/colorteller:latest"
	UDPImage         = "networking-e2e-test-images/udp-echoserver:latest"
	GRPCImage        = "networking-e2e-test-images/grpc-echoserver:latest"
)
