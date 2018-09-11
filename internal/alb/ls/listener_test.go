package ls

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albacm"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	extensions "k8s.io/api/extensions/v1beta1"
)

const (
	newARN   = "arn1"
	newTg    = "tg1"
	newPort  = 8080
	newProto = elbv2.ProtocolEnumHttp
	newPort2 = 9000
)

var (
	mockList1 *elbv2.Listener
	mockList2 *elbv2.Listener
	mockList3 *elbv2.Listener
	rOpts1    *ReconcileOptions
)

func init() {
	albelbv2.ELBV2svc = albelbv2.NewDummy()
	albacm.ACMsvc = albacm.NewDummy()
	albcache.NewCache(metric.DummyCollector{})

	rOpts1 = &ReconcileOptions{
		TargetGroups:    tg.TargetGroups{tg.DummyTG("tg1", "service")},
		LoadBalancerArn: nil,
		Eventf:          func(a, b, c string, d ...interface{}) {},
	}
}

func setup() {
	albelbv2.ELBV2svc = albelbv2.NewDummy()

	mockList1 = &elbv2.Listener{
		Port:     aws.Int64(newPort),
		Protocol: aws.String(elbv2.ProtocolEnumHttp),
		DefaultActions: []*elbv2.Action{{
			Type:           aws.String("default"),
			TargetGroupArn: aws.String(newTg),
		}},
	}

	mockList2 = &elbv2.Listener{
		Port:     aws.Int64(newPort2),
		Protocol: aws.String(elbv2.ProtocolEnumHttp),
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
	ing := dummy.NewIngress()

	tgs, _ := tg.NewDesiredTargetGroups(&tg.NewDesiredTargetGroupsOptions{
		Ingress:        ing,
		LoadBalancerID: "lbid",
		Store:          store.NewDummy(),
		CommonTags:     util.ELBv2Tags{},
		Logger:         log.New("logger"),
	})

	o := &NewDesiredListenerOptions{
		Port:         loadbalancer.PortData{desiredPort, elbv2.ProtocolEnumHttp},
		Logger:       log.New("test"),
		Ingress:      ing,
		TargetGroups: tgs,
	}

	l, _ := NewDesiredListener(o)

	desiredProto := elbv2.ProtocolEnumHttp
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
	ing := dummy.NewIngress()
	tgs, _ := tg.NewDesiredTargetGroups(&tg.NewDesiredTargetGroupsOptions{
		Ingress:        ing,
		LoadBalancerID: "lbid",
		Store:          store.NewDummy(),
		CommonTags:     util.ELBv2Tags{},
		Logger:         log.New("logger"),
	})

	o := &NewDesiredListenerOptions{
		Ingress:        ing,
		Port:           loadbalancer.PortData{desiredPort, "HTTPS"},
		CertificateArn: desiredCertArn,
		SslPolicy:      desiredSslPolicy,
		Logger:         log.New("test"),
		TargetGroups:   tgs,
	}

	l, _ := NewDesiredListener(o)

	desiredProto := elbv2.ProtocolEnumHttp
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
		logger:         log.New("test"),
		ls:             ls{desired: mockList1},
		defaultBackend: &extensions.IngressBackend{ServiceName: "service", ServicePort: intstr.FromInt(newPort)},
	}

	m := mockList1
	m.ListenerArn = aws.String(createdARN)

	albelbv2.ELBV2svc.SetField("CreateListenerOutput", &elbv2.CreateListenerOutput{
		Listeners: []*elbv2.Listener{m},
	})

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

	albelbv2.ELBV2svc.SetField("DeleteListenerOutput", &elbv2.DeleteListenerOutput{})

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
		logger:         log.New("test"),
		defaultBackend: &extensions.IngressBackend{ServiceName: "service", ServicePort: intstr.FromInt(newPort)},
		ls: ls{
			desired: mockList2,
			current: mockList1,
		},
	}

	m := mockList2
	m.ListenerArn = aws.String(listenerArn)

	albelbv2.ELBV2svc.SetField("ModifyListenerOutput", &elbv2.ModifyListenerOutput{Listeners: []*elbv2.Listener{m}})

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
		logger:         log.New("test"),
		defaultBackend: &extensions.IngressBackend{ServiceName: "service", ServicePort: intstr.FromInt(newPort)},
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
		logger:         log.New("test"),
		defaultBackend: &extensions.IngressBackend{ServiceName: "service", ServicePort: intstr.FromInt(newPort)},
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
		logger:         log.New("test"),
		defaultBackend: &extensions.IngressBackend{ServiceName: "service", ServicePort: intstr.FromInt(newPort)},
		ls: ls{
			desired: mockList1,
			current: mockList1,
		},
	}

	if lNoMod.needsModification(nil) {
		t.Error("Listener reported modification needed. Desired and Current were the same")
	}

	lCertNeedsMod := Listener{
		logger:         log.New("test"),
		defaultBackend: &extensions.IngressBackend{ServiceName: "service", ServicePort: intstr.FromInt(newPort)},
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

func Test_domainMatchesIngressTLSHost(t *testing.T) {
	var tests = []struct {
		domain string
		host   string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "exampl0.com", false},

		// wildcards
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.exampl0.com", "foo.example.com", false},

		// invalid hosts, not sure these are possible
		{"*.*.example.com", "foo.bar.example.com", false},
		{"foo.*.example.com", "foo.bar.example.com", false},
	}

	for _, test := range tests {
		var msg = "should"
		if !test.want {
			msg = "should not"
		}

		t.Run(fmt.Sprintf("%s %s match %s", test.domain, msg, test.host), func(t *testing.T) {
			have := domainMatchesIngressTLSHost(aws.String(test.domain), aws.String(test.host))
			if test.want != have {
				t.Fail()
			}
		})
	}
}

func Test_getCertificates(t *testing.T) {
	var tests = []struct {
		name      string
		arn       *string
		ingress   *extensions.Ingress
		result    *acm.ListCertificatesOutput
		resultErr error
		expected  int
	}{
		{
			name: "when ACM has exact match",
			ingress: &extensions.Ingress{
				Spec: extensions.IngressSpec{
					TLS: []extensions.IngressTLS{
						{
							Hosts: []string{"foo.example.com"},
						},
					},
				},
			},
			result: &acm.ListCertificatesOutput{
				CertificateSummaryList: []*acm.CertificateSummary{
					{
						CertificateArn: aws.String("arn:acm:xxx:yyy:zzz/kkk:www"),
						DomainName:     aws.String("foo.example.com"),
					},
				},
			},
			expected: 1,
		}, {
			name: "when ACM has wildcard match",
			ingress: &extensions.Ingress{
				Spec: extensions.IngressSpec{
					TLS: []extensions.IngressTLS{
						{
							Hosts: []string{"foo.example.com"},
						},
					},
				},
			},
			result: &acm.ListCertificatesOutput{
				CertificateSummaryList: []*acm.CertificateSummary{
					{
						CertificateArn: aws.String("arn:acm:xxx:yyy:zzz/kkk:www"),
						DomainName:     aws.String("*.example.com"),
					},
				},
			},
			expected: 1,
		}, {
			name: "when ACM has multiple matches",
			ingress: &extensions.Ingress{
				Spec: extensions.IngressSpec{
					TLS: []extensions.IngressTLS{
						{
							Hosts: []string{"foo.example.com"},
						},
					},
				},
			},
			result: &acm.ListCertificatesOutput{
				CertificateSummaryList: []*acm.CertificateSummary{
					{
						CertificateArn: aws.String("arn:acm:xxx:yyy:zzz/kkk:www"),
						DomainName:     aws.String("foo.example.com"),
					},
					{
						CertificateArn: aws.String("arn:acm:xxx:yyy:zzz/kkk:mmm"),
						DomainName:     aws.String("*.example.com"),
					},
				},
			},
			expected: 2,
		}, {
			name: "when certificate-arn is set in annotation",
			arn:  aws.String("arn:acm:xxx:yyy:zzz/kkk:www"),
			// this result list is a fake, as we're not actually going to ACM in this case
			result: &acm.ListCertificatesOutput{
				CertificateSummaryList: []*acm.CertificateSummary{
					{
						CertificateArn: aws.String("arn:acm:xxx:yyy:zzz/kkk:www"),
						DomainName:     aws.String("foo.example.com"),
					},
				},
			},
			expected: 1,
		}, {
			name:      "when ACM returns error",
			ingress:   &extensions.Ingress{},
			resultErr: fmt.Errorf("oh no!"),
			expected:  0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logger = log.New(test.name)
			albacm.ACMsvc.(*albacm.Dummy).SetField("ListCertificatesOutput", test.result)
			albacm.ACMsvc.(*albacm.Dummy).SetField("ListCertificatesError", test.resultErr)

			certificates, err := getCertificates(test.arn, test.ingress, logger)
			if test.resultErr != err {
				t.Error(err)
			}

			if len(certificates) != test.expected {
				t.Errorf("Expected %d, got %d certificates in result", test.expected, len(certificates))
			}
		})
	}
}
