package listener

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awselb "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	"github.com/coreos/alb-ingress-controller/pkg/util/types"
)

const (
	newARN   = "arn1"
	newTg    = "tg1"
	newPort  = 8080
	newProto = "HTTP"
)

var (
	logr *log.Logger
)

func init() {
	logr = log.New("test")
}

func TestNewHTTPListener(t *testing.T) {
	desiredPort := int64(newPort)
	o := &NewDesiredListenerOptions{
		Port:   desiredPort,
		Logger: logr,
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
		Logger:         logr,
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
	elbv2.ELBV2API
}

func (m mockELBV2Client) CreateListener(*awselb.CreateListenerInput) (*awselb.CreateListenerOutput, error) {
	o := &awselb.CreateListenerOutput{
		Listeners: []*awselb.Listener{
			{
				Port:        aws.Int64(newPort),
				ListenerArn: aws.String(newARN),
				Protocol:    aws.String(newProto),
			}},
	}
	return o, nil
}

func mockEventf(a, b, c string, d ...interface{}) {
}

// TestReconcileCreate calls Reconcile on a mock Listener instance and assures creation is
// attempted.
func TestReconcileCreate(t *testing.T) {
	elbv2.ELBV2svc = mockELBV2Client{}

	dl := &awselb.Listener{
		Port:     aws.Int64(8080),
		Protocol: aws.String("HTTP"),
		DefaultActions: []*awselb.Action{{
			Type:           aws.String("default"),
			TargetGroupArn: aws.String(newTg),
		}},
	}

	l := Listener{
		logger:  logr,
		Desired: dl,
	}

	rOpts := &ReconcileOptions{
		TargetGroups:    nil,
		LoadBalancerArn: nil,
		Eventf:          mockEventf,
	}

	l.Reconcile(rOpts)

	if *l.Current.ListenerArn != newARN {
		t.Errorf("Listener arn not properly set. Actual: %s, Expected: %s", *l.Current.ListenerArn, newARN)
	}
	if types.DeepEqual(l.Desired, l.Current) {
		t.Error("After creation, desired and current listeners did not match.")
	}
}
