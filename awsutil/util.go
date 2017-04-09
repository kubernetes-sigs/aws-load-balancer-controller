package awsutil

import (
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	prometheus.MustRegister(OnUpdateCount)
	prometheus.MustRegister(ReloadCount)
	prometheus.MustRegister(AWSErrorCount)
	prometheus.MustRegister(ManagedIngresses)
	prometheus.MustRegister(AWSCache)
	prometheus.MustRegister(AWSRequest)
}

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

// Prettify wraps github.com/aws/aws-sdk-go/aws/awsutil.Prettify. Preventing the need to import it
// in each package.
func Prettify(i interface{}) string {
	return awsutil.Prettify(i)
}

// DeepEqual wraps github.com/aws/aws-sdk-go/aws/awsutil.Prettify. Preventing the need to import it
// in each package.
func DeepEqual(a interface{}, b interface{}) bool {
	return awsutil.DeepEqual(a, b)
}
