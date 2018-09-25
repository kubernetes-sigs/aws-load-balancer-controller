package albelbv2

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

type Dummy struct {
	elbv2iface.ELBV2API
	resp interface{}

	outputs output
}

type output map[string]interface{}

func (o output) error(s string) error {
	if v, ok := o[s]; ok && v != nil {
		return v.(error)
	}
	return nil
}

func NewDummy() *Dummy {
	d := &Dummy{}
	d.outputs = make(output)
	return d
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
func (d *Dummy) DescribeTargetGroupTargetsForArn(arn *string) (TargetDescriptions, error) {
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
	return d.outputs["CreateListenerOutput"].(*elbv2.CreateListenerOutput), d.outputs.error("CreateListenerError")
}

// ModifyListener ...
func (d *Dummy) ModifyListener(in *elbv2.ModifyListenerInput) (*elbv2.ModifyListenerOutput, error) {
	return d.outputs["ModifyListenerOutput"].(*elbv2.ModifyListenerOutput), d.outputs.error("ModifyListenerError")
}

// CreateRule ...
func (d *Dummy) CreateRule(in *elbv2.CreateRuleInput) (*elbv2.CreateRuleOutput, error) {
	return d.outputs["CreateRuleOutput"].(*elbv2.CreateRuleOutput), d.outputs.error("CreateRuleError")
}

// ModifyRule ...
func (d *Dummy) ModifyRule(in *elbv2.ModifyRuleInput) (*elbv2.ModifyRuleOutput, error) {
	fmt.Println(d.outputs.error("ModifyRuleError"))
	return d.outputs["ModifyRuleOutput"].(*elbv2.ModifyRuleOutput), d.outputs.error("ModifyRuleError")
}

// DeleteRule ...
func (d *Dummy) DeleteRule(in *elbv2.DeleteRuleInput) (*elbv2.DeleteRuleOutput, error) {
	return d.outputs["DeleteRuleOutput"].(*elbv2.DeleteRuleOutput), d.outputs.error("DeleteRuleError")
}

// GetLoadBalancerByArn ...
func (d *Dummy) GetLoadBalancerByArn(arn string) (*elbv2.LoadBalancer, error) {
	return d.outputs["GetLoadBalancerByArn"].(*elbv2.LoadBalancer), d.outputs.error("GetLoadBalancerByArn")
}

// SetField ...
func (d *Dummy) SetField(field string, v interface{}) {
	d.outputs[field] = v
}
