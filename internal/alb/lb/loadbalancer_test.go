package lb

import (
	"os"
	"testing"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	clusterName    = "cluster1"
	ingressName    = "ingress1"
	sg1            = "sg-123"
	sg2            = "sg-abc"
	tag1Key        = "tag1"
	tag1Value      = "value1"
	tag2Key        = "tag2"
	tag2Value      = "value2"
	webACL         = "current-id-of-web-acl"
	expectedWebACL = "new-id-of-web-acl"
)

var (
	logr         *log.Logger
	lbScheme     *string
	lbTags       types.ELBv2Tags
	lbTags2      types.ELBv2Tags
	expectedName string
	existing     *elbv2.LoadBalancer
	lbOpts       *NewCurrentLoadBalancerOptions
	expectedWeb  *string
	currentWeb   *string
)

func init() {
	logr = log.New("test")
	lbScheme = aws.String("internal")
	lbTags = types.ELBv2Tags{
		{
			Key:   aws.String(tag1Key),
			Value: aws.String(tag1Value),
		},
		{
			Key:   aws.String(tag2Key),
			Value: aws.String(tag2Value),
		},
	}

	albcache.NewCache(metric.DummyCollector{})

	expectedName = createLBName(api.NamespaceDefault, ingressName, clusterName)
	// setting expectedName initially for clarity. Will be overwritten with a bad name below
	existing = &elbv2.LoadBalancer{
		LoadBalancerArn:  aws.String("arn"),
		LoadBalancerName: aws.String(expectedName),
	}
	lbTags2 = types.ELBv2Tags{
		{
			Key:   aws.String("IngressName"),
			Value: aws.String(ingressName),
		},
		{
			Key:   aws.String("Namespace"),
			Value: aws.String(api.NamespaceDefault),
		},
	}

	currentWeb = aws.String(webACL)
	expectedWeb = aws.String(expectedWebACL)
	lbOpts = &NewCurrentLoadBalancerOptions{
		LoadBalancer: existing,
		Logger:       logr,
	}
}

func buildIngress() *extensions.Ingress {
	ports := []int64{
		int64(80),
		int64(443),
		int64(8080),
	}
	hosts := []string{
		"1.test.domain",
		"2.test.domain",
		"3.test.domain",
	}
	paths := []string{
		"/",
		"/store",
		"/store/dev",
	}
	svcs := []string{
		"1service",
		"2service",
		"3service",
	}
	svcPorts := []int{
		30001,
		30002,
		30003,
	}

	ing := &extensions.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      ingressName,
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

func TestNewDesiredLoadBalancer(t *testing.T) {
	dummyStore := &store.Dummy{}
	cfg := dummyStore.GetConfig()
	cfg.ALBNamePrefix = clusterName
	dummyStore.SetConfig(cfg)

	ing := buildIngress()
	dummyStore.GetIngressAnnotationsResponse = annotations.NewIngressDummy()
	dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Scheme = lbScheme
	dummyStore.GetIngressAnnotationsResponse.LoadBalancer.SecurityGroups = types.AWSStringSlice{aws.String(sg1), aws.String(sg2)}
	dummyStore.GetIngressAnnotationsResponse.LoadBalancer.WebACLId = expectedWeb

	dummyStore.GetServiceAnnotationsResponse = annotations.NewServiceDummy()

	lbOpts := &NewDesiredLoadBalancerOptions{
		Ingress:    ing,
		Logger:     logr,
		CommonTags: lbTags,
		Store:      dummyStore,
	}

	os.Setenv("AWS_VPC_ID", "vpc-id")
	expectedID := createLBName(api.NamespaceDefault, ingressName, clusterName)
	l, err := NewDesiredLoadBalancer(lbOpts)
	if err != nil {
		t.Error(err.Error())
	}

	key1, _ := l.tags.desired.Get(tag1Key)
	switch {
	case *l.lb.desired.LoadBalancerName != expectedID:
		t.Errorf("LB ID was wrong. Expected: %s | Actual: %s", expectedID, l.id)
	case *l.lb.desired.Scheme != *lbScheme:
		t.Errorf("LB scheme was wrong. Expected: %s | Actual: %s", *lbScheme, *l.lb.desired.Scheme)
	case *l.lb.desired.SecurityGroups[0] == sg2: // note sgs are sorted during checking for modification needs.
		t.Errorf("Security group was wrong. Expected: %s | Actual: %s", sg2, *l.lb.desired.SecurityGroups[0])
	case key1 != tag1Value:
		t.Errorf("Tag was invalid. Expected: %s | Actual: %s", tag1Value, key1)
	case *l.options.desired.webACLId != *expectedWeb:
		t.Errorf("Web ACL ID was invalid. Expected: %s | Actual: %s", *expectedWeb, *l.options.desired.webACLId)

	}
}

// Temporarily disabled until we mock out the AWS API calls involved
// func TestNewCurrentLoadBalancer(t *testing.T) {
// 	l, err := NewCurrentLoadBalancer(lbOpts)
// 	if err != nil {
// 		t.Errorf("Failed to create LoadBalancer object from existing elbv2.LoadBalancer."+
// 			"Error: %s", err.Error())
// 		return
// 	}

// 	switch {
// 	case *l.lb.current.LoadBalancerName != expectedName:
// 		t.Errorf("Current LB created returned improper LoadBalancerName. Expected: %s | "+
// 			"Desired: %s", expectedName, *l.lb.current.LoadBalancerName)
// 	case *l.options.current.webACLId != *currentWeb:
// 		t.Errorf("Current LB created returned improper Web ACL Id. Expected: %s | "+
// 			"Desired: %s", *currentWeb, *l.options.current.webACLId)
// 	}
// }

// TestLoadBalancerFailsWithInvalidName ensures an error is returned when the LoadBalancerName does
// match what would have been calculated for the LB from the clustername, ingressname, and
// namespace
