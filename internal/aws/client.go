package aws

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/ticketmaster/aws-sdk-go-cache/cache"
	"k8s.io/apiserver/pkg/server/healthz"
)

// Initialize the global AWS clients.
// TODO, pass these aws clients instances to controller instead of global clients.
// But due to huge number of aws clients, it's best to have one container AWS client that embed these aws clients.
func Initialize(AWSAPIMaxRetries int, AWSAPIDebug bool, clusterName string, mc metric.Collector, cc *cache.Config) {
	sess := NewSession(&aws.Config{MaxRetries: aws.Int(AWSAPIMaxRetries)}, AWSAPIDebug, mc, cc)
	NewCloudsvc(sess)
}

type AWSHealthChecker struct{}

var _ healthz.HealthzChecker = (*AWSHealthChecker)(nil)

func (c *AWSHealthChecker) Name() string {
	// TODO, can we rename it to, e.g. AWS? is there any dependencies on the name?
	return "aws-alb-ingress-controller"
}

// TODO, validate the call health check frequency
func (c *AWSHealthChecker) Check(_ *http.Request) error {
	for _, fn := range []func() error{
		Cloudsvc.StatusACM(),
		Cloudsvc.StatusEC2(),
		Cloudsvc.StatusIAM(),
	} {
		err := fn()
		if err != nil {
			glog.Errorf("Controller health check failed: %v", err.Error())
			return err
		}
	}
	return nil
}
