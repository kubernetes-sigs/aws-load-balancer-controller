package lb

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
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
	wafACL         = "current-id-of-waf-acl"
	expectedWAFACL = "new-id-of-waf-acl"
)

var (
	logr         *log.Logger
	lbScheme     *string
	lbTags       types.Tags
	lbTags2      types.Tags
	expectedName string
	existing     *elbv2.LoadBalancer
	lbOpts       *NewCurrentLoadBalancerOptions
	expectedWaf  *string
	currentWaf   *string
)

func init() {
	logr = log.New("test")
	lbScheme = aws.String("internal")
	lbTags = types.Tags{
		{
			Key:   aws.String(tag1Key),
			Value: aws.String(tag1Value),
		},
		{
			Key:   aws.String(tag2Key),
			Value: aws.String(tag2Value),
		},
	}

	expectedName = createLBName(namespace, ingressName, clusterName)
	// setting expectedName initially for clarity. Will be overwritten with a bad name below
	existing = &elbv2.LoadBalancer{
		LoadBalancerName: aws.String(expectedName),
	}
	lbTags2 = types.Tags{
		{
			Key:   aws.String("IngressName"),
			Value: aws.String(ingressName),
		},
		{
			Key:   aws.String("Namespace"),
			Value: aws.String(namespace),
		},
	}

	currentWaf = aws.String(wafACL)
	expectedWaf = aws.String(expectedWAFACL)
	lbOpts = &NewCurrentLoadBalancerOptions{
		LoadBalancer:  existing,
		Logger:        logr,
		Tags:          lbTags2,
		ALBNamePrefix: clusterName,
		WafACLID:      currentWaf,
	}
}

func TestNewDesiredLoadBalancer(t *testing.T) {
	anno := &annotations.Annotations{
		Scheme:         lbScheme,
		SecurityGroups: types.AWSStringSlice{aws.String(sg1), aws.String(sg2)},
		WafACLID:       expectedWaf,
	}

	lbOpts := &NewDesiredLoadBalancerOptions{
		ALBNamePrefix:        clusterName,
		Namespace:            namespace,
		Logger:               logr,
		Annotations:          anno,
		Tags:                 lbTags,
		IngressName:          ingressName,
		ExistingLoadBalancer: &LoadBalancer{},
	}

	expectedID := createLBName(namespace, ingressName, clusterName)
	l, err := NewDesiredLoadBalancer(lbOpts)
	fmt.Println(err)

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
	case *l.options.desired.wafACLID != *expectedWaf:
		t.Errorf("WAF ACL ID was invalid. Expected: %s | Actual: %s", *expectedWaf, *l.options.desired.wafACLID)

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
// 	case *l.options.current.wafACLID != *currentWaf:
// 		t.Errorf("Current LB created returned improper WAF ACL Id. Expected: %s | "+
// 			"Desired: %s", *currentWaf, *l.options.current.wafACLID)
// 	}
// }

// TestLoadBalancerFailsWithInvalidName ensures an error is returned when the LoadBalancerName does
// match what would have been calculated for the LB from the clustername, ingressname, and
// namespace
func TestLoadBalancerFailsWithInvalidName(t *testing.T) {
	// overwriting the expectName to ensure it fails
	existing.LoadBalancerName = aws.String("BADNAME")
	l, err := NewCurrentLoadBalancer(lbOpts)
	if err == nil {
		t.Errorf("LB creation should have failed due to improper name. Expected: %s | "+
			"Actual: %s.", expectedName, *l.lb.current.LoadBalancerName)
	}
}
