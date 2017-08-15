package aws

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	"github.com/karlseguin/ccache"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	logger = log.New("aws")
	prometheus.MustRegister(OnUpdateCount)
	prometheus.MustRegister(ReloadCount)
	prometheus.MustRegister(AWSErrorCount)
	prometheus.MustRegister(ManagedIngresses)
	prometheus.MustRegister(AWSCache)
	prometheus.MustRegister(AWSRequest)
}

type APICache struct {
	cache *ccache.Cache
}

var (
	// Session is a pointer to the AWS session
	Session *session.Session
	// ALBsvc is a pointer to the awsutil ELBV2 service
	ALBsvc *ELBV2
	// Ec2svc is a pointer to the awsutil EC2 service
	Ec2svc *EC2
	// ACMsvc is a pointer to the awsutil ACM service
	ACMsvc *ACM
	// IAMsvc is a pointer to the awsutil IAM service
	IAMsvc *IAM
	// AWSDebug turns on AWS API debug logging
	AWSDebug bool

	logger *log.Logger

	// OnUpdateCount is a counter of the controller OnUpdate calls
	OnUpdateCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "albingress_updates",
		Help: "Number of times OnUpdate has been called.",
	})

	// ReloadCount is a counter of the controller Reload calls
	ReloadCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "albingress_reloads",
		Help: "Number of times Reload has been called.",
	})

	// AWSErrorCount is a counter of AWS errors
	AWSErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_aws_errors",
		Help: "Number of errors from the AWS API",
	},
		[]string{"service", "operation"})

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

// NewSession returns an AWS session based off of the provided AWS config
func NewSession(awsconfig *aws.Config) *session.Session {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "AWS", "request": "NewSession"}).Add(float64(1))
		logger.Errorf("Failed to create AWS session: %s", err.Error())
		return nil
	}

	session.Handlers.Send.PushFront(func(r *request.Request) {
		AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if AWSDebug {
			logger.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

	session.Handlers.Complete.PushFront(func(r *request.Request) {
		if r.Error != nil {
			AWSErrorCount.With(
				prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		}
	})
	return session
}

// DeepEqual wraps github.com/aws/aws-sdk-go/aws/awsutil.DeepEqual. Preventing the need to import it
// in each package.
func DeepEqual(a interface{}, b interface{}) bool {
	return awsutil.DeepEqual(a, b)
}

// Prettify wraps github.com/aws/aws-sdk-go/aws/awsutil.Prettify. Preventing the need to import it
// in each package.
func Prettify(a interface{}) string {
	return awsutil.Prettify(a)
}

// Get retrieves a key in the API cache. If they key doesn't exist or it expired, nil is returned.
func (ac APICache) Get(key string) *ccache.Item {
	i := ac.cache.Get(key)
	if i == nil || i.Expired() {
		return nil
	}
	return i
}

// Set add a key and value to the API cache.
func (ac APICache) Set(key string, value interface{}, duration time.Duration) {
	ac.cache.Set(key, value, duration)
}
