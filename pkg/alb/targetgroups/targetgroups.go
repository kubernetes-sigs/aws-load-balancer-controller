package targetgroups

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroup"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
	extensions "k8s.io/api/extensions/v1beta1"
)

// TargetGroups is a slice of TargetGroup pointers
type TargetGroups []*targetgroup.TargetGroup

// LookupBySvc returns the position of a TargetGroup by its SvcName, returning -1 if unfound.
func (t TargetGroups) LookupBySvc(svc string) int {
	for p, v := range t {
		if v.SvcName == svc {
			return p
		}
	}
	// LOG: log.Infof("No TG matching service found. SVC %s", "controller", svc)
	return -1
}

// Find returns the position of a TargetGroup by its ID, returning -1 if unfound.
func (t TargetGroups) Find(tg *targetgroup.TargetGroup) int {
	for p, v := range t {
		if *v.ID == *tg.ID {
			return p
		}
	}
	return -1
}

// Reconcile kicks off the state synchronization for every target group inside this TargetGroups
// instance.
func (t TargetGroups) Reconcile(rOpts *ReconcileOptions) (TargetGroups, error) {
	var output TargetGroups
	for _, tg := range t {
		tgOpts := targetgroup.NewReconcileOptions()
		tgOpts.SetEventf(rOpts.Eventf)
		tgOpts.SetVpcID(rOpts.VpcID)
		if err := tg.Reconcile(tgOpts); err != nil {
			return nil, err
		}
		if !tg.Deleted {
			output = append(output, tg)
		}
	}

	return output, nil
}

// StripDesiredState removes the DesiredTags, DesiredTargetGroup, and DesiredTargets from all TargetGroups
func (t TargetGroups) StripDesiredState() {
	for _, targetgroup := range t {
		targetgroup.DesiredTags = nil
		targetgroup.DesiredTargetGroup = nil
		targetgroup.DesiredTargets = nil
	}
}

// NewTargetGroupsFromAWSTargetGroups returns a new targetgroups.TargetGroups based on an elbv2.TargetGroups.
func NewTargetGroupsFromAWSTargetGroups(targetGroups []*elbv2.TargetGroup, clusterName string, loadBalancerID *string, logger *log.Logger) (TargetGroups, error) {
	var output TargetGroups

	for _, targetGroup := range targetGroups {
		tags, err := awsutil.ALBsvc.DescribeTagsForArn(targetGroup.TargetGroupArn)
		if err != nil {
			return nil, err
		}

		tg, err := targetgroup.NewTargetGroupFromAWSTargetGroup(targetGroup, tags, clusterName, *loadBalancerID, logger)
		if err != nil {
			return nil, err
		}

		logger.Infof("Fetching Targets for Target Group %s", *targetGroup.TargetGroupArn)

		targets, err := awsutil.ALBsvc.DescribeTargetGroupTargetsForArn(targetGroup.TargetGroupArn)
		if err != nil {
			return nil, err
		}
		tg.CurrentTargets = targets
		output = append(output, tg)
	}

	return output, nil
}

type NewTargetGroupsFromIngressOptions struct {
	Ingress              *extensions.Ingress
	LoadBalancerID       *string
	ExistingTargetGroups TargetGroups
	Annotations          *annotations.Annotations
	ClusterName          *string
	Namespace            string
	Tags                 util.Tags
	Logger               *log.Logger
	GetServiceNodePort   func(string, int32) (*int64, error)
	GetNodes             func() util.AWSStringSlice
}

// NewTargetGroupsFromIngress returns a new targetgroups.TargetGroups based on an extensions.Ingress.
func NewTargetGroupsFromIngress(o *NewTargetGroupsFromIngressOptions) (TargetGroups, error) {
	output := o.ExistingTargetGroups

	for _, rule := range o.Ingress.Spec.Rules {
		for _, path := range rule.HTTP.Paths {

			serviceKey := fmt.Sprintf("%s/%s", o.Namespace, path.Backend.ServiceName)
			port, err := o.GetServiceNodePort(serviceKey, path.Backend.ServicePort.IntVal)
			if err != nil {
				return nil, err
			}

			// Start with a new target group with a new Desired state.
			targetGroup := targetgroup.NewTargetGroup(o.Annotations, o.Tags, o.ClusterName, o.LoadBalancerID, port, o.Logger, path.Backend.ServiceName)
			targetGroup.DesiredTargets = o.GetNodes()

			// If this target group is already defined, copy the desired state over
			if i := output.Find(targetGroup); i >= 0 {
				output[i].DesiredTags = targetGroup.DesiredTags
				output[i].DesiredTargetGroup = targetGroup.DesiredTargetGroup
				output[i].DesiredTargets = targetGroup.DesiredTargets
				continue
			}

			output = append(output, targetGroup)
		}
	}
	return output, nil
}

type ReconcileOptions struct {
	LoadBalancerTargetGroups *TargetGroups
	Eventf                   func(string, string, string, ...interface{})
	VpcID                    *string
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetTargetGroups(targetgroups *TargetGroups) *ReconcileOptions {
	r.LoadBalancerTargetGroups = targetgroups
	return r
}

func (r *ReconcileOptions) SetVpcID(vpcid *string) *ReconcileOptions {
	r.VpcID = vpcid
	return r
}

func (r *ReconcileOptions) SetEventf(f func(string, string, string, ...interface{})) *ReconcileOptions {
	r.Eventf = f
	return r
}
