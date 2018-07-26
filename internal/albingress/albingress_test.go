package albingress

import (
	"os"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var a *ALBIngress

var (
	logger   *log.Logger
	ports    []int64
	schemes  []bool
	hosts    []string
	paths    []string
	svcs     []string
	svcPorts []int
)

func init() {
	albcache.NewCache(metric.DummyCollector{})
	os.Setenv("AWS_VPC_ID", "vpc-id")
	logger = log.New("test")
	ports = []int64{
		int64(80),
		int64(443),
		int64(8080),
	}
	schemes = []bool{
		false,
		true,
		false,
	}
	hosts = []string{
		"1.test.domain",
		"2.test.domain",
		"3.test.domain",
	}
	paths = []string{
		"/",
		"/store",
		"/store/dev",
	}
	svcs = []string{
		"1service",
		"2service",
		"3service",
	}
	svcPorts = []int{
		30001,
		30002,
		30003,
	}
}

func buildIngress() *extensions.Ingress {
	ing := &extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: api.NamespaceDefault,
		},
		Spec: extensions.IngressSpec{
			Backend: &extensions.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	for i := range ports {
		extRules := extensions.IngressRule{
			Host: hosts[i],
			IngressRuleValue: extensions.IngressRuleValue{
				HTTP: &extensions.HTTPIngressRuleValue{
					Paths: []extensions.HTTPIngressPath{{
						Path: paths[i],
						Backend: extensions.IngressBackend{
							ServiceName: svcs[i],
							ServicePort: intstr.FromInt(svcPorts[i]),
						},
					},
					},
				},
			},
		}
		ing.Spec.Rules = append(ing.Spec.Rules, extRules)
	}
	return ing
}

func TestNewALBIngressFromIngress(t *testing.T) {
	ing := buildIngress()
	store := store.NewDummy()

	options := &NewALBIngressFromIngressOptions{
		Ingress: ing,
		Store:   store,
	}
	ingress := NewALBIngressFromIngress(options)
	if ingress == nil {
		t.Errorf("NewALBIngressFromIngress returned nil")
	}
}
