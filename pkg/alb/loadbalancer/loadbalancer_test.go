package loadbalancer

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	"github.com/coreos/alb-ingress-controller/pkg/util/types"
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
	tags         types.Tags
	tags2        types.Tags
	expectedName string
	existing     *elbv2.LoadBalancer
	opts         *NewCurrentLoadBalancerOptions
	expectedWaf  *string
	currentWaf   *string
)

func init() {
	logr = log.New("test")
	lbScheme = aws.String("internal")
	tags = types.Tags{
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
	tags2 = types.Tags{
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
	opts = &NewCurrentLoadBalancerOptions{
		LoadBalancer:  existing,
		Logger:        logr,
		Tags:          tags2,
		ALBNamePrefix: clusterName,
		WafACL:        currentWaf,
	}
}

func TestNewDesiredLoadBalancer(t *testing.T) {
	anno := &annotations.Annotations{
		Scheme:         lbScheme,
		SecurityGroups: types.AWSStringSlice{aws.String(sg1), aws.String(sg2)},
		WafAclId:       expectedWaf,
	}

	opts := &NewDesiredLoadBalancerOptions{
		ALBNamePrefix: clusterName,
		Namespace:     namespace,
		Logger:        logr,
		Annotations:   anno,
		Tags:          tags,
		IngressName:   ingressName,
	}

	expectedID := createLBName(namespace, ingressName, clusterName)
	lb := NewDesiredLoadBalancer(opts)

	key1, _ := lb.DesiredTags.Get(tag1Key)
	switch {
	case *lb.Desired.LoadBalancerName != expectedID:
		t.Errorf("LB ID was wrong. Expected: %s | Actual: %s", expectedID, lb.ID)
	case *lb.Desired.Scheme != *lbScheme:
		t.Errorf("LB scheme was wrong. Expected: %s | Actual: %s", *lbScheme, *lb.Desired.Scheme)
	case *lb.Desired.SecurityGroups[0] == sg2: // note sgs are sorted during checking for modification needs.
		t.Errorf("Security group was wrong. Expected: %s | Actual: %s", sg2, *lb.Desired.SecurityGroups[0])
	case key1 != tag1Value:
		t.Errorf("Tag was invalid. Expected: %s | Actual: %s", tag1Value, key1)
	case *lb.DesiredWafAcl != *expectedWaf:
		t.Errorf("WAF ACL ID was invalid. Expected: %s | Actual: %s", *expectedWaf, *lb.DesiredWafAcl)

	}
}

func TestNewCurrentLoadBalancer(t *testing.T) {
	lb, err := NewCurrentLoadBalancer(opts)
	if err != nil {
		t.Errorf("Failed to create LoadBalancer object from existing elbv2.LoadBalancer."+
			"Error: %s", err.Error())
		return
	}

	switch {
	case *lb.Current.LoadBalancerName != expectedName:
		t.Errorf("Current LB created returned improper LoadBalancerName. Expected: %s | "+
			"Desired: %s", expectedName, *lb.Current.LoadBalancerName)
	case *lb.CurrentWafAcl != *currentWaf:
		t.Errorf("Current LB created returned improper WAF ACL Id. Expected: %s | "+
			"Desired: %s", *currentWaf, *lb.CurrentWafAcl)
	}
}

// TestLoadBalancerFailsWithInvalidName ensures an error is returned when the LoadBalancerName does
// match what would have been calculated for the LB from the clustername, ingressname, and
// namespace
func TestLoadBalancerFailsWithInvalidName(t *testing.T) {
	// overwriting the expectName to ensure it fails
	existing.LoadBalancerName = aws.String("BADNAME")
	lb, err := NewCurrentLoadBalancer(opts)
	if err == nil {
		t.Errorf("LB creation should have failed due to improper name. Expected: %s | "+
			"Actual: %s.", expectedName, *lb.Current.LoadBalancerName)
	}
}
