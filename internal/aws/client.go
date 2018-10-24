package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albacm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albiam"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albsession"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albwafregional"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/ticketmaster/aws-sdk-go-cache/cache"
	"k8s.io/apiserver/pkg/server/healthz"
	"net/http"
)

// Initialize the global AWS clients.
// TODO, pass these aws clients instances to controller instead of global clients.
// But due to huge number of aws clients, it's best to have one container AWS client that embed these aws clients.
func Initialize(AWSAPIMaxRetries int, AWSAPIDebug bool, clusterName string, mc metric.Collector, cc *cache.Config) {
	sess := albsession.NewSession(&aws.Config{MaxRetries: aws.Int(AWSAPIMaxRetries)}, AWSAPIDebug, mc, cc)
	albelbv2.NewELBV2(sess)
	albec2.NewEC2(sess)
	albec2.NewEC2Metadata(sess)
	albacm.NewACM(sess)
	albiam.NewIAM(sess)
	albrgt.NewRGT(sess, clusterName)
	albwafregional.NewWAFRegional(sess)
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
		albacm.ACMsvc.Status(),
		albec2.EC2svc.Status(),
		albelbv2.ELBV2svc.Status(),
		albiam.IAMsvc.Status(),
	} {
		err := fn()
		if err != nil {
			glog.Errorf("Controller health check failed: %v", err.Error())
			return err
		}
	}
	return nil
}
