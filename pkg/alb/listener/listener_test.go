package listener

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

var (
	logger *log.Logger
)

func init() {
	logger = log.New("test")
}

func TestNewHTTPListener(t *testing.T) {
	desiredPort := int64(8080)
	o := &NewListenerOptions{
		Port:   desiredPort,
		Logger: logger,
	}

	l := NewListener(o)

	desiredProto := "HTTP"
	if o.CertificateArn != nil {
		desiredProto = "HTTPS"
	}
	switch {
	case *l.DesiredListener.Port != desiredPort:
		t.Errorf("Invalid port created. Actual: %d | Expected: %d", *l.DesiredListener.Port,
			desiredPort)
	case *l.DesiredListener.Protocol != desiredProto:
		t.Errorf("Invalid protocol created. Actual: %s | Expected: %s",
			*l.DesiredListener.Protocol, desiredProto)
	}
}

func TestNewHTTPSListener(t *testing.T) {
	desiredPort := int64(443)
	desiredCertArn := aws.String("abc123")
	o := &NewListenerOptions{
		Port:           desiredPort,
		CertificateArn: desiredCertArn,
		Logger:         logger,
	}

	l := NewListener(o)

	desiredProto := "HTTP"
	if o.CertificateArn != nil {
		desiredProto = "HTTPS"
	}
	switch {
	case *l.DesiredListener.Port != desiredPort:
		t.Errorf("Invalid port created. Actual: %d | Expected: %d", *l.DesiredListener.Port,
			desiredPort)
	case *l.DesiredListener.Protocol != desiredProto:
		t.Errorf("Invalid protocol created. Actual: %s | Expected: %s",
			*l.DesiredListener.Protocol, desiredProto)
	case *l.DesiredListener.Certificates[0].CertificateArn != *desiredCertArn:
		t.Errorf("Invalid certificate ARN. Actual: %s | Expected: %s",
			*l.DesiredListener.Certificates[0].CertificateArn, *desiredCertArn)
	}
}

type mockELBV2Client struct {
	elbv2.ELBV2
}

/*func TestReconcileCreate(t *testing.T) {

	elbv2.ELBV2svc = mockELBV2Client{}

	tg :=
		targetgroup.TargetGroup{
			SvcName: "service1",
		}
	tgs := targetgroups.TargetGroups{

	}

	rOpts := ReconcileOptions{
		TargetGroups:    nil,
		LoadBalancerArn: nil,
		Eventf:          nil,
	}
}*/
