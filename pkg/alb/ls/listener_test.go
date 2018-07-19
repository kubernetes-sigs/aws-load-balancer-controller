package ls

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	newARN   = "arn1"
	newTg    = "tg1"
	newPort  = 8080
	newProto = "HTTP"
	newPort2 = 9000
)

var (
	logr      *log.Logger
	mockList1 *elbv2.Listener
	mockList2 *elbv2.Listener
	mockList3 *elbv2.Listener
	rOpts1    *ReconcileOptions
)

func init() {
	albelbv2.ELBV2svc = mockELBV2Client{}
	logr = log.New("test")

	rOpts1 = &ReconcileOptions{
		TargetGroups:    nil,
		LoadBalancerArn: nil,
		Eventf:          mockEventf,
	}
}

func setup() {
	mockList1 = &elbv2.Listener{
		Port:     aws.Int64(newPort),
		Protocol: aws.String("HTTP"),
		DefaultActions: []*elbv2.Action{{
			Type:           aws.String("default"),
			TargetGroupArn: aws.String(newTg),
		}},
	}

	mockList2 = &elbv2.Listener{
		Port:     aws.Int64(newPort2),
		Protocol: aws.String("HTTP"),
		DefaultActions: []*elbv2.Action{{
			Type:           aws.String("default"),
			TargetGroupArn: aws.String(newTg),
		}},
	}

	mockList3 = &elbv2.Listener{
		Port:     aws.Int64(newPort),
		Protocol: aws.String("HTTPS"),
		Certificates: []*elbv2.Certificate{
			{CertificateArn: aws.String("abc")},
		},
		DefaultActions: []*elbv2.Action{{
			Type:           aws.String("default"),
			TargetGroupArn: aws.String(newTg),
		}},
		SslPolicy: aws.String("ELBSecurityPolicy-TLS-1-2-2017-01"),
	}
}

func TestNewHTTPListener(t *testing.T) {
	desiredPort := int64(newPort)
	o := &NewDesiredListenerOptions{
		Port:   annotations.PortData{desiredPort, "HTTP"},
		Logger: logr,
	}

	l, _ := NewDesiredListener(o)

	desiredProto := "HTTP"
	if o.CertificateArn != nil {
		desiredProto = "HTTPS"
	}
	switch {
	case *l.ls.desired.Port != desiredPort:
		t.Errorf("Invalid port created. Actual: %d | Expected: %d", *l.ls.desired.Port,
			desiredPort)
	case *l.ls.desired.Protocol != desiredProto:
		t.Errorf("Invalid protocol created. Actual: %s | Expected: %s",
			*l.ls.desired.Protocol, desiredProto)
	}
}

func TestNewHTTPSListener(t *testing.T) {
	desiredPort := int64(443)
	desiredCertArn := aws.String("abc123")
	desiredSslPolicy := aws.String("ELBSecurityPolicy-Test")
	o := &NewDesiredListenerOptions{
		Port:           annotations.PortData{desiredPort, "HTTPS"},
		CertificateArn: desiredCertArn,
		SslPolicy:      desiredSslPolicy,
		Logger:         logr,
	}

	l, _ := NewDesiredListener(o)

	desiredProto := "HTTP"
	if o.CertificateArn != nil {
		desiredProto = "HTTPS"
	}
	switch {
	case *l.ls.desired.Port != desiredPort:
		t.Errorf("Invalid port created. Actual: %d | Expected: %d", *l.ls.desired.Port,
			desiredPort)
	case *l.ls.desired.Protocol != desiredProto:
		t.Errorf("Invalid protocol created. Actual: %s | Expected: %s",
			*l.ls.desired.Protocol, desiredProto)
	case *l.ls.desired.Certificates[0].CertificateArn != *desiredCertArn:
		t.Errorf("Invalid certificate ARN. Actual: %s | Expected: %s",
			*l.ls.desired.Certificates[0].CertificateArn, *desiredCertArn)
	case *l.ls.desired.SslPolicy != *desiredSslPolicy:
		t.Errorf("Invalid certificate SSL Policy. Actual: %s | Expected: %s",
			*l.ls.desired.SslPolicy, *desiredSslPolicy)
	}
}

type mockELBV2Client struct {
	albelbv2.ELBV2API
}

func (m mockELBV2Client) CreateListener(*elbv2.CreateListenerInput) (*elbv2.CreateListenerOutput, error) {
	o := &elbv2.CreateListenerOutput{
		Listeners: []*elbv2.Listener{
			{
				Port:        aws.Int64(newPort),
				ListenerArn: aws.String(newARN),
				Protocol:    aws.String(newProto),
			}},
	}
	return o, nil
}

func (m mockELBV2Client) RemoveListener(*string) error {
	return nil
}

func (m mockELBV2Client) ModifyListener(*elbv2.ModifyListenerInput) (*elbv2.ModifyListenerOutput, error) {
	o := &elbv2.ModifyListenerOutput{
		Listeners: []*elbv2.Listener{
			{
				Port:        aws.Int64(newPort2),
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
	setup()
	l := Listener{
		logger: logr,
		ls:     ls{desired: mockList1},
	}

	l.Reconcile(rOpts1)

	if *l.ls.current.ListenerArn != newARN {
		t.Errorf("Listener arn not properly set. Actual: %s, Expected: %s", *l.ls.current.ListenerArn, newARN)
	}
	if types.DeepEqual(l.ls.desired, l.ls.current) {
		t.Error("After creation, desired and current listeners did not match.")
	}
}

// TestReconcileDelete calls Reconcile on a mock Listener instance and assures deletion is
// attempted.
func TestReconcileDelete(t *testing.T) {
	setup()
	albelbv2.ELBV2svc = mockELBV2Client{}

	l := Listener{
		logger: logr,
		ls:     ls{current: mockList1},
	}

	l.Reconcile(rOpts1)

	if !l.deleted {
		t.Error("Listener was deleted deleted flag was not set to true.")
	}

}

// TestReconcileModify calls Reconcile on a mock Listener instance and assures modification is
// attempted when the ports between a desired and current listener differ.
func TestReconcileModifyPortChange(t *testing.T) {
	setup()
	l := Listener{
		logger: logr,
		ls: ls{
			desired: mockList2,
			current: mockList1,
		},
	}

	l.Reconcile(rOpts1)

	if *l.ls.current.Port != *l.ls.desired.Port {
		t.Errorf("Error. Current: %d | Desired: %d", *l.ls.current.Port, *l.ls.desired.Port)
	}
	if *l.ls.current.ListenerArn != newARN {
		t.Errorf("Listener arn not properly set. Actual: %s, Expected: %s", *l.ls.current.ListenerArn, newARN)
	}

}

// TestReconcileModify calls Reconcile on a mock Listener that contains an identical current and
// desired state. It expects no operation to be taken.
func TestReconcileModifyNoChange(t *testing.T) {
	setup()
	l := Listener{
		logger: logr,
		ls: ls{
			desired: mockList2,
			current: mockList1,
		},
	}

	l.ls.desired.Port = mockList1.Port // this sets ports identical. Should prevent failure, if removed, test should fail.
	l.Reconcile(rOpts1)

	if *l.ls.current.Port != *mockList1.Port {
		t.Errorf("Error. Current: %d | Desired: %d", *l.ls.current.Port, *mockList1.Port)
	}
}

// TestModificationNeeds sends different listeners through to see if a modification is needed.
func TestModificationNeeds(t *testing.T) {
	setup()
	lPortNeedsMod := Listener{
		logger: logr,
		ls: ls{
			desired: mockList2,
			current: mockList1,
		},
	}

	if !lPortNeedsMod.needsModification(nil) {
		t.Error("Listener reported no modification needed. Ports were different and should" +
			"require modification")
	}

	lNoMod := Listener{
		logger: logr,
		ls: ls{
			desired: mockList1,
			current: mockList1,
		},
	}

	if lNoMod.needsModification(nil) {
		t.Error("Listener reported modification needed. Desired and Current were the same")
	}

	lCertNeedsMod := Listener{
		logger: logr,
		ls: ls{
			desired: mockList3,
			current: mockList1,
		},
	}

	if !lCertNeedsMod.needsModification(nil) {
		t.Error("Listener reported no modification needed. Certificates were different and" +
			"should require modification")
	}
}
