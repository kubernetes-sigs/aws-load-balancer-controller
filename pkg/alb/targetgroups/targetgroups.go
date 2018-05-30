package targetgroups

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroup"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
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

// FindById returns the position of a TargetGroup by its ID, returning -1 if unfound.
func (t TargetGroups) FindById(id string) (int, *targetgroup.TargetGroup) {
	for p, v := range t {
		if v.ID == id {
			return p, v
		}
	}
	return -1, nil
}

// FindCurrentByARN returns the position of a current TargetGroup and the TargetGroup itself based on the ARN passed. Returns the position of -1 if unfound.
func (t TargetGroups) FindCurrentByARN(id string) (int, *targetgroup.TargetGroup) {
	for p, v := range t {
		if *v.Current.TargetGroupArn == id {
			return p, v
		}
	}
	return -1, nil
}

// Reconcile kicks off the state synchronization for every target group inside this TargetGroups
// instance. It returns the new TargetGroups its created and a list of TargetGroups it believes
// should be cleaned up.
func (t TargetGroups) Reconcile(rOpts *ReconcileOptions) (TargetGroups, TargetGroups, error) {
	var output TargetGroups
	var cleanUp TargetGroups
	for _, tg := range t {
		tgOpts := &targetgroup.ReconcileOptions{
			Eventf:            rOpts.Eventf,
			VpcID:             rOpts.VpcID,
			ManagedSGInstance: rOpts.ManagedSGInstance,
		}
		if err := tg.Reconcile(tgOpts); err != nil {
			return nil, nil, err
		}
		if tg.Deleted {
			cleanUp = append(cleanUp, tg)
		}
		output = append(output, tg)
	}

	return output, cleanUp, nil
}

// StripDesiredState removes the Tags.Desired, DesiredTargetGroup, and Targets.Desired from all TargetGroups
func (t TargetGroups) StripDesiredState() {
	for _, targetgroup := range t {
		targetgroup.Tags.Desired = nil
		targetgroup.Desired = nil
		targetgroup.Targets.Desired = nil
	}
}

type NewCurrentTargetGroupsOptions struct {
	TargetGroups   []*elbv2.TargetGroup
	ALBNamePrefix  string
	LoadBalancerID string
	Logger         *log.Logger
}

// NewCurrentTargetGroups returns a new targetgroups.TargetGroups based on an elbv2.TargetGroups.
func NewCurrentTargetGroups(o *NewCurrentTargetGroupsOptions) (TargetGroups, error) {
	var output TargetGroups

	for _, targetGroup := range o.TargetGroups {
		tags, err := albelbv2.ELBV2svc.DescribeTagsForArn(targetGroup.TargetGroupArn)
		if err != nil {
			return nil, err
		}

		tg, err := targetgroup.NewCurrentTargetGroup(&targetgroup.NewCurrentTargetGroupOptions{
			TargetGroup:    targetGroup,
			Tags:           tags,
			ALBNamePrefix:  o.ALBNamePrefix,
			LoadBalancerID: o.LoadBalancerID,
			Logger:         o.Logger,
		})
		if err != nil {
			return nil, err
		}

		o.Logger.Infof("Fetching Targets for Target Group %s", *targetGroup.TargetGroupArn)

		current, err := albelbv2.ELBV2svc.DescribeTargetGroupTargetsForArn(targetGroup.TargetGroupArn, []*elbv2.TargetDescription{})
		if err != nil {
			return nil, err
		}
		tg.Targets.Current = current
		output = append(output, tg)
	}

	return output, nil
}

type NewDesiredTargetGroupsOptions struct {
	Ingress              *extensions.Ingress
	LoadBalancerID       string
	ExistingTargetGroups TargetGroups
	Annotations          *annotations.Annotations
	ALBNamePrefix        string
	Namespace            string
	Tags                 util.Tags
	Logger               *log.Logger
	GetServiceNodePort   func(string, int32) (*int64, error)
	GetNodes             func() util.AWSStringSlice
}

// NewDesiredTargetGroups returns a new targetgroups.TargetGroups based on an extensions.Ingress.
func NewDesiredTargetGroups(o *NewDesiredTargetGroupsOptions) (TargetGroups, error) {
	output := o.ExistingTargetGroups

	for _, rule := range o.Ingress.Spec.Rules {
		for _, path := range rule.HTTP.Paths {

			serviceKey := fmt.Sprintf("%s/%s", o.Namespace, path.Backend.ServiceName)
			port, err := o.GetServiceNodePort(serviceKey, path.Backend.ServicePort.IntVal)
			if err != nil {
				return nil, err
			}

			// Start with a new target group with a new Desired state.
			targetGroup := targetgroup.NewDesiredTargetGroup(&targetgroup.NewDesiredTargetGroupOptions{
				Annotations:    o.Annotations,
				Tags:           o.Tags,
				ALBNamePrefix:  o.ALBNamePrefix,
				LoadBalancerID: o.LoadBalancerID,
				Port:           *port,
				Logger:         o.Logger,
				SvcName:        path.Backend.ServiceName,
			})
			targetGroup.Targets.Desired = o.GetNodes()

			// If this target group is already defined, copy the desired state over
			if i, tg := output.FindById(targetGroup.ID); i >= 0 {
				targets := []*elbv2.TargetDescription{}
				for _, instanceID := range targetGroup.Targets.Desired {
					targets = append(targets, &elbv2.TargetDescription{
						Id:   instanceID,
						Port: port,
					})
				}

				// Only set with current TargetGroup Arn if current target group exists
				desired := targetGroup.Targets.Desired
				if tg.Current != nil {
					desired, err = albelbv2.ELBV2svc.DescribeTargetGroupTargetsForArn(tg.Current.TargetGroupArn, targets)
					if err != nil {
						return nil, err
					}
				}

				tg.Tags.Desired = targetGroup.Tags.Desired
				tg.Desired = targetGroup.Desired
				tg.Targets.Desired = desired
				continue
			}

			output = append(output, targetGroup)
		}
	}
	return output, nil
}

type ReconcileOptions struct {
	Eventf            func(string, string, string, ...interface{})
	VpcID             *string
	ManagedSGInstance *string
}
