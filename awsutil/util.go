package awsutil

import "github.com/prometheus/client_golang/prometheus"

var (
	// Route53svc is a pointer to the awsutil Route53 service
	Route53svc *Route53
	// Elbv2svc is a pointer to the awsutil ELBV2 service
	Elbv2svc *ELBV2
	// Ec2svc is a pointer to the awsutil EC2 service
	Ec2svc *EC2
	// AWSDebug turns on AWS API debug logging
	AWSDebug bool

	// OnUpdateCount is a counter of the controller OnUpdate calls
	OnUpdateCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "albingress_updates",
		Help: "Number of times OnUpdate has been called.",
	},
	)

	// ReloadCount is a counter of the controller Reload calls
	ReloadCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "albingress_reloads",
		Help: "Number of times Reload has been called.",
	},
	)

	// AWSErrorCount is a counter of AWS errors
	AWSErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_aws_errors",
		Help: "Number of errors from the AWS API",
	},
		[]string{"service", "request"},
	)

	// ManagedIngresses contains the current tally of managed ingresses
	ManagedIngresses = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "albingress_managed_ingresses",
		Help: "Number of ingresses being managed",
	})

	// AWSCache contains the hits and misses to our caches
	AWSCache = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_cache",
		Help: "Number of ingresses being managed",
	},
		[]string{"cache", "action"})

	// AWSRequest contains the requests made to the AWS API
	AWSRequest = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_aws_requests",
		Help: "Number of requests made to the AWS API",
	},
		[]string{"service", "operation"})
)

func init() {
	prometheus.MustRegister(OnUpdateCount)
	prometheus.MustRegister(ReloadCount)
	prometheus.MustRegister(AWSErrorCount)
	prometheus.MustRegister(ManagedIngresses)
	prometheus.MustRegister(AWSCache)
	prometheus.MustRegister(AWSRequest)
}
