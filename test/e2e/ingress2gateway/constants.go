package ingress2gateway

const (
	ingressController = "ingress.k8s.aws/alb"

	annotationScheme              = "alb.ingress.kubernetes.io/scheme"
	annotationTargetType          = "alb.ingress.kubernetes.io/target-type"
	annotationGroupName           = "alb.ingress.kubernetes.io/group.name"
	annotationGroupOrder          = "alb.ingress.kubernetes.io/group.order"
	annotationTags                = "alb.ingress.kubernetes.io/tags"
	annotationHealthCheckPath     = "alb.ingress.kubernetes.io/healthcheck-path"
	annotationHealthyThreshold    = "alb.ingress.kubernetes.io/healthy-threshold-count"
	annotationUnhealthyThreshold  = "alb.ingress.kubernetes.io/unhealthy-threshold-count"
	annotationHealthCheckInterval = "alb.ingress.kubernetes.io/healthcheck-interval-seconds"
	annotationIPAddressType       = "alb.ingress.kubernetes.io/ip-address-type"
	annotationDryRunPlan          = "alb.ingress.kubernetes.io/dry-run-plan"
	annotationListenPorts         = "alb.ingress.kubernetes.io/listen-ports"

	gwDryRunAnnotation = "gateway.k8s.aws/dry-run"
	gwDryRunPlan       = "gateway.k8s.aws/dry-run-plan"
	migrationTagKey    = "gateway.k8s.aws/migrated-from"

	hostAdmin = "admin.example.com"
	hostApp   = "app.example.com"
	hostAPI   = "api.example.com"

	pathRoot   = "/"
	pathAPI    = "/api"
	pathHealth = "/health"

	bodyServiceA = "service-a"
	bodyServiceB = "service-b"
	bodyServiceC = "service-c"
)
