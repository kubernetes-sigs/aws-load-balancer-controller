package utils

// UDP/GRPC image sources; InitGatewayFramework selects public (default) or mirror per --resolve-image-tags.
const (
	PublicUDPImage  = "public.ecr.aws/u6k2n8q7/nixozach/udp-echoserver:latest"
	PublicGRPCImage = "public.ecr.aws/u6k2n8q7/nixozach/grpc-echoserver:latest"
	MirrorUDPImage  = "networking-e2e-test-images/udp-echoserver:latest"
	MirrorGRPCImage = "networking-e2e-test-images/grpc-echoserver:latest"
)

// Test image paths; vars so downstream can override with resolved (non-`:latest`) tags.
var (
	HelloImage       = "networking-e2e-test-images/hello-multi:latest"
	ColortellerImage = "networking-e2e-test-images/colorteller:latest"
	UDPImage         = PublicUDPImage
	GRPCImage        = PublicGRPCImage
)
