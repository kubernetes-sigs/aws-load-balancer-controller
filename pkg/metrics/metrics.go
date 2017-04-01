package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	OnUpdateCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "albingress_updates",
		Help: "Number of times OnUpdate has been called.",
	},
	)
	AWSErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_aws_errors",
		Help: "Number of errors from the AWS API",
	},
		[]string{"service", "request"},
	)
	ManagedIngresses = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "albingress_managed_ingresses",
		Help: "Number of ingresses being managed",
	})
	AWSCache = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_cache",
		Help: "Number of ingresses being managed",
	},
		[]string{"cache", "action"})
	AWSRequest = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_aws_requests",
		Help: "Number of requests made to the AWS API",
	},
		[]string{"service", "operation"})
)

func init() {
	prometheus.MustRegister(OnUpdateCount)
	prometheus.MustRegister(AWSErrorCount)
	prometheus.MustRegister(ManagedIngresses)
	prometheus.MustRegister(AWSCache)
	prometheus.MustRegister(AWSRequest)
}
