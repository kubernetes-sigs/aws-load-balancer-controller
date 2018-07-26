package ls

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
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
	mockList1 *elbv2.Listener
	mockList2 *elbv2.Listener
	mockList3 *elbv2.Listener
	rOpts1    *ReconcileOptions
)

func init() {
	albelbv2.ELBV2svc = &albelbv2.Dummy{}
	albcache.NewCache(metric.DummyCollector{})

	rOpts1 = &ReconcileOptions{
		TargetGroups:    nil,
		LoadBalancerArn: nil,
		Eventf:          func(a, b, c string, d ...interface{}) {},
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
	ing := store.NewDummyIngress()
	o := &NewDesiredListenerOptions{
		Port:    loadbalancer.PortData{desiredPort, "HTTP"},
		Logger:  log.New("test"),
		Ingress: ing,
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
	ing := store.NewDummyIngress()
	o := &NewDesiredListenerOptions{
		Ingress:        ing,
		Port:           loadbalancer.PortData{desiredPort, "HTTPS"},
		CertificateArn: desiredCertArn,
		SslPolicy:      desiredSslPolicy,
		Logger:         log.New("test"),
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

// TestReconcileCreate calls Reconcile on a mock Listener instance and assures creation is
// attempted.
func TestReconcileCreate(t *testing.T) {
	setup()

	createdARN := "listener arn"
	l := Listener{
		logger: log.New("test"),
		ls:     ls{desired: mockList1},
	}

	m := mockList1
	m.ListenerArn = aws.String(createdARN)
	resp := &elbv2.CreateListenerOutput{
		Listeners: []*elbv2.Listener{m},
	}

	albelbv2.ELBV2svc.SetResponse(resp, nil)

	err := l.Reconcile(rOpts1)
	if err != nil {
		t.Error(err)
	}

	if *l.ls.current.ListenerArn != createdARN {
		t.Errorf("Listener arn not properly set. Actual: %s, Expected: %s", *l.ls.current.ListenerArn, createdARN)
	}
	if !types.DeepEqual(l.ls.desired, l.ls.current) {
		t.Error("After creation, desired and current listeners did not match.")
	}
}

// TestReconcileDelete calls Reconcile on a mock Listener instance and assures deletion is
// attempted.
func TestReconcileDelete(t *testing.T) {
	setup()

	l := Listener{
		logger: log.New("test"),
		ls:     ls{current: mockList1},
	}

	albelbv2.ELBV2svc.SetResponse(&elbv2.DeleteListenerOutput{}, nil)

	l.Reconcile(rOpts1)

	if !l.deleted {
		t.Error("Listener was deleted deleted flag was not set to true.")
	}

}

// TestReconcileModify calls Reconcile on a mock Listener instance and assures modification is
// attempted when the ports between a desired and current listener differ.
func TestReconcileModifyPortChange(t *testing.T) {
	setup()

	listenerArn := "listener arn"
	l := Listener{
		logger: log.New("test"),
		ls: ls{
			desired: mockList2,
			current: mockList1,
		},
	}

	m := mockList2
	m.ListenerArn = aws.String(listenerArn)
	resp := &elbv2.ModifyListenerOutput{
		Listeners: []*elbv2.Listener{m},
	}

	albelbv2.ELBV2svc.SetResponse(resp, nil)

	l.Reconcile(rOpts1)

	if *l.ls.current.Port != *l.ls.desired.Port {
		t.Errorf("Error. Current: %d | Desired: %d", *l.ls.current.Port, *l.ls.desired.Port)
	}
	if *l.ls.current.ListenerArn != listenerArn {
		t.Errorf("Listener arn not properly set. Actual: %s, Expected: %s", *l.ls.current.ListenerArn, listenerArn)
	}

}

// TestReconcileModify calls Reconcile on a mock Listener that contains an identical current and
// desired state. It expects no operation to be taken.
func TestReconcileModifyNoChange(t *testing.T) {
	setup()
	l := Listener{
		logger: log.New("test"),
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
		logger: log.New("test"),
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
		logger: log.New("test"),
		ls: ls{
			desired: mockList1,
			current: mockList1,
		},
	}

	if lNoMod.needsModification(nil) {
		t.Error("Listener reported modification needed. Desired and Current were the same")
	}

	lCertNeedsMod := Listener{
		logger: log.New("test"),
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
