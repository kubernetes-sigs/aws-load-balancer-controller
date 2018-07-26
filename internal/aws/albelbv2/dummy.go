package albelbv2

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

type Dummy struct {
	elbv2iface.ELBV2API
	resp      interface{}
	respError error
}

// CacheDelete ...
func (d *Dummy) CacheDelete(string, string) {
	return
}

// ClusterLoadBalancers ...
func (d *Dummy) ClusterLoadBalancers() ([]*elbv2.LoadBalancer, error) {
	return nil, nil
}

// ClusterTargetGroups ...
func (d *Dummy) ClusterTargetGroups() (map[string][]*elbv2.TargetGroup, error) {
	return nil, nil
}

// UpdateTags ...
func (d *Dummy) UpdateTags(arn *string, old util.ELBv2Tags, new util.ELBv2Tags) error { return nil }

// RemoveTargetGroup ...
func (d *Dummy) RemoveTargetGroup(arn *string) error { return nil }

// DescribeTargetGroupTargetsForArn ...
func (d *Dummy) DescribeTargetGroupTargetsForArn(arn *string, targets ...TargetDescriptions) (TargetDescriptions, error) {
	return nil, nil
}

// RemoveListener ...
func (d *Dummy) RemoveListener(arn *string) error { return nil }

// DescribeListenersForLoadBalancer ...
func (d *Dummy) DescribeListenersForLoadBalancer(loadBalancerArn *string) ([]*elbv2.Listener, error) {
	return nil, nil
}

// Status ...
func (d *Dummy) Status() func() error { return nil }

// DescribeLoadBalancerAttributesFiltered ...
func (d *Dummy) DescribeLoadBalancerAttributesFiltered(*string) (LoadBalancerAttributes, error) {
	return nil, nil
}

// DescribeTargetGroupAttributesFiltered ...
func (d *Dummy) DescribeTargetGroupAttributesFiltered(*string) (TargetGroupAttributes, error) {
	return nil, nil
}

// CreateListener ...
func (d *Dummy) CreateListener(in *elbv2.CreateListenerInput) (*elbv2.CreateListenerOutput, error) {
	return d.resp.(*elbv2.CreateListenerOutput), d.respError
}

// ModifyListener ...
func (d *Dummy) ModifyListener(in *elbv2.ModifyListenerInput) (*elbv2.ModifyListenerOutput, error) {
	return d.resp.(*elbv2.ModifyListenerOutput), d.respError
}

// SetResponse ...
func (d *Dummy) SetResponse(i interface{}, e error) {
	d.resp = i
	d.respError = e
}
