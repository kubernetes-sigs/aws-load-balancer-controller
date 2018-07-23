package lb

import (
	"os"
	"testing"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	clusterName    = "cluster1"
	namespace      = "namespace1"
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

	expectedName = createLBName(namespace, ingressName, clusterName)
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
			Value: aws.String(namespace),
		},
	}

	currentWeb = aws.String(webACL)
	expectedWeb = aws.String(expectedWebACL)
	lbOpts = &NewCurrentLoadBalancerOptions{
		LoadBalancer:  existing,
		Logger:        logr,
		ALBNamePrefix: clusterName,
	}
}

func TestNewDesiredLoadBalancer(t *testing.T) {
	anno := &annotations.Ingress{
		LoadBalancer: &loadbalancer.Config{
			Scheme:         lbScheme,
			SecurityGroups: types.AWSStringSlice{aws.String(sg1), aws.String(sg2)},
			WebACLId:       expectedWeb,
		},
	}

	lbOpts := &NewDesiredLoadBalancerOptions{
		ALBNamePrefix:        clusterName,
		Namespace:            namespace,
		Logger:               logr,
		IngressAnnotations:   anno,
		CommonTags:           lbTags,
		IngressName:          ingressName,
		ExistingLoadBalancer: &LoadBalancer{},
		Ingress: &extensions.Ingress{
			Spec: extensions.IngressSpec{},
		},
	}

	os.Setenv("AWS_VPC_ID", "vpc-id")
	expectedID := createLBName(namespace, ingressName, clusterName)
	l, _ := NewDesiredLoadBalancer(lbOpts)

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
