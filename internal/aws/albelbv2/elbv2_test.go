package albelbv2

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

type mockedELBV2DescribeLoadBalancers struct {
	elbv2iface.ELBV2API
	Resp elbv2.DescribeLoadBalancersOutput
}

// func (m mockedELBV2DescribeLoadBalancers) DescribeLoadBalancersRequest(input *elbv2.DescribeLoadBalancersInput) (*request.Request, *elbv2.DescribeLoadBalancersOutput) {
// 	// r := request.New(aws.Config{}, nil, nil, nil, nil, nil, nil)
// 	// return &r, &m.Resp
// 	return &request.Request{
// 		HTTPRequest: &http.Request{},
// 		Operation:   &request.Operation{},
// 	}, &m.Resp
// }

// func TestClusterLoadBalancers(t *testing.T) {
// 	loadBalancers := []*elbv2.LoadBalancer{
// 		{LoadBalancerArn: aws.String("arn1")},
// 		{LoadBalancerArn: aws.String("arn2")},
// 	}

// 	cases := []struct {
// 		Resp         elbv2.DescribeLoadBalancersOutput
// 		ResourceTags albrgt.Resources
// 		Expected     []*elbv2.LoadBalancer
// 	}{
// 		{
// 			Resp:         elbv2.DescribeLoadBalancersOutput{LoadBalancers: loadBalancers},
// 			ResourceTags: albrgt.Resources{LoadBalancers: map[string]util.ELBv2Tags{"arn1": nil}},
// 			Expected: []*elbv2.LoadBalancer{
// 				{LoadBalancerArn: aws.String("arn1")},
// 			},
// 		},
// 		{
// 			Resp:         elbv2.DescribeLoadBalancersOutput{LoadBalancers: loadBalancers},
// 			ResourceTags: albrgt.Resources{LoadBalancers: map[string]util.ELBv2Tags{"arn miss": nil}},
// 			Expected:     []*elbv2.LoadBalancer{},
// 		},
// 	}

// 	for i, c := range cases {
// 		e := ELBV2{mockedELBV2DescribeLoadBalancers{Resp: c.Resp}}
// 		loadbalancers, err := e.ClusterLoadBalancers(&c.ResourceTags)
// 		if err != nil {
// 			t.Fatalf("%v: unexpected error %v", i, err)
// 		}
// 		if a, e := len(loadbalancers), len(c.Expected); a != e {
// 			t.Fatalf("%v: expected %d load balancers, got %d", i, e, a)
// 		}
// 		for j, loadbalancer := range loadbalancers {
// 			if a, e := loadbalancer, c.Expected[j]; *a.LoadBalancerArn != *e.LoadBalancerArn {
// 				t.Errorf("%v: expected %v loadbalancer, got %v", i, e, a)
// 			}
// 		}
// 	}
// }

func TestSortLoadBalancerAttributes(t *testing.T) {
	key1 := "hello"
	value1 := "world"
	key2 := "other"
	value2 := "value"
	key3 := "something"
	value3 := "else"
	attributes1 := LoadBalancerAttributes{
		&elbv2.LoadBalancerAttribute{
			Key:   &key2,
			Value: &value2,
		},
		&elbv2.LoadBalancerAttribute{
			Key:   &key1,
			Value: &value1,
		},
	}.Sorted()
	attributes2 := LoadBalancerAttributes{
		&elbv2.LoadBalancerAttribute{
			Key:   &key1,
			Value: &value1,
		},
		&elbv2.LoadBalancerAttribute{
			Key:   &key2,
			Value: &value2,
		},
	}.Sorted()
	if log.Prettify(attributes1) != log.Prettify(attributes2) {
		t.Errorf("LoadBalancerAttribute sort failed, expected attributes to be inequal.")
	}
	attributes2 = LoadBalancerAttributes{
		&elbv2.LoadBalancerAttribute{
			Key:   &key1,
			Value: &value1,
		},
		&elbv2.LoadBalancerAttribute{
			Key:   &key2,
			Value: &value2,
		},
		&elbv2.LoadBalancerAttribute{
			Key:   &key3,
			Value: &value3,
		},
	}.Sorted()
	if log.Prettify(attributes1) == log.Prettify(attributes2) {
		t.Errorf("LoadBalancerAttribute sort failed, expected attributes to be equal.")
	}
}
