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
	o := &NewDesiredListenerOptions{
		Port:   desiredPort,
		Logger: logger,
	}

	l := NewDesiredListener(o)

	desiredProto := "HTTP"
	if o.CertificateArn != nil {
		desiredProto = "HTTPS"
	}
	switch {
	case *l.Desired.Port != desiredPort:
		t.Errorf("Invalid port created. Actual: %d | Expected: %d", *l.Desired.Port,
			desiredPort)
	case *l.Desired.Protocol != desiredProto:
		t.Errorf("Invalid protocol created. Actual: %s | Expected: %s",
			*l.Desired.Protocol, desiredProto)
	}
}

func TestNewHTTPSListener(t *testing.T) {
	desiredPort := int64(443)
	desiredCertArn := aws.String("abc123")
	o := &NewDesiredListenerOptions{
		Port:           desiredPort,
		CertificateArn: desiredCertArn,
		Logger:         logger,
	}

	l := NewDesiredListener(o)

	desiredProto := "HTTP"
	if o.CertificateArn != nil {
		desiredProto = "HTTPS"
	}
	switch {
	case *l.Desired.Port != desiredPort:
		t.Errorf("Invalid port created. Actual: %d | Expected: %d", *l.Desired.Port,
			desiredPort)
	case *l.Desired.Protocol != desiredProto:
		t.Errorf("Invalid protocol created. Actual: %s | Expected: %s",
			*l.Desired.Protocol, desiredProto)
	case *l.Desired.Certificates[0].CertificateArn != *desiredCertArn:
		t.Errorf("Invalid certificate ARN. Actual: %s | Expected: %s",
			*l.Desired.Certificates[0].CertificateArn, *desiredCertArn)
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
