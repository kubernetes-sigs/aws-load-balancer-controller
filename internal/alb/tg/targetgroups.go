package tg

import (
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"

	"github.com/aws/aws-sdk-go/service/elbv2"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// LookupByBackend returns the position of a TargetGroup by its ingress backend, returning -1 if unfound.
func (t TargetGroups) LookupByBackend(backend extensions.IngressBackend) int {
	for p, v := range t {
		if v == nil {
			continue
		}

		if v.SvcName == backend.ServiceName && v.tg.desired != nil && v.SvcPort.String() == backend.ServicePort.String() {
			return p
		}
	}
	// LOG: log.Infof("No TG matching service found. SVC %s", "controller", svc)
	return -1
}

// FindById returns the position of a TargetGroup by its ID, returning -1 if unfound.
func (t TargetGroups) FindById(id string) (int, *TargetGroup) {
	for p, v := range t {
		if v.ID == id {
			return p, v
		}
	}
	return -1, nil
}

// FindCurrentByARN returns the position of a current TargetGroup and the TargetGroup itself based on the ARN passed. Returns the position of -1 if unfound.
func (t TargetGroups) FindCurrentByARN(id string) (int, *TargetGroup) {
	for p, v := range t {
		if v.CurrentARN() != nil && *v.CurrentARN() == id {
			return p, v
		}
	}
	return -1, nil
}

// Reconcile kicks off the state synchronization for every target group inside this TargetGroups
// instance. It returns the new TargetGroups its created and a list of TargetGroups it believes
// should be cleaned up.
func (t TargetGroups) Reconcile(rOpts *ReconcileOptions) (TargetGroups, error) {
	var output TargetGroups

	for _, tg := range t {
		if err := tg.Reconcile(rOpts); err != nil {
			return nil, err
		}

		if !tg.deleted {
			output = append(output, tg)
		}
	}

	return output, nil
}

// StripDesiredState removes the Tags.Desired, DesiredTargetGroup, and Targets.Desired from all TargetGroups
func (t TargetGroups) StripDesiredState() {
	for _, targetgroup := range t {
		targetgroup.StripDesiredState()
	}
}

type NewCurrentTargetGroupsOptions struct {
	TargetGroups   []*elbv2.TargetGroup
	LoadBalancerID string
	Logger         *log.Logger
}

// NewCurrentTargetGroups returns a new targetgroups.TargetGroups based on an elbv2.TargetGroups.
func NewCurrentTargetGroups(o *NewCurrentTargetGroupsOptions) (TargetGroups, error) {
	var output TargetGroups

	for _, targetGroup := range o.TargetGroups {
		tg, err := NewCurrentTargetGroup(&NewCurrentTargetGroupOptions{
			TargetGroup:    targetGroup,
			LoadBalancerID: o.LoadBalancerID,
			Logger:         o.Logger,
		})
		if err != nil {
			return nil, err
		}
		o.Logger.Infof("Fetching Targets for Target Group %s", *targetGroup.TargetGroupArn)

		current, err := albelbv2.ELBV2svc.DescribeTargetGroupTargetsForArn(targetGroup.TargetGroupArn)
		if err != nil {
			return nil, err
		}
		tg.targets.current = current

		v, err := albelbv2.ELBV2svc.DescribeTargetGroupAttributes(&elbv2.DescribeTargetGroupAttributesInput{TargetGroupArn: targetGroup.TargetGroupArn})
		if err != nil {
			return nil, err
		}
		tg.attributes.current = v.Attributes

		output = append(output, tg)
	}

	return output, nil
}

type NewDesiredTargetGroupsOptions struct {
	Ingress              *extensions.Ingress
	LoadBalancerID       string
	ExistingTargetGroups TargetGroups
	Store                store.Storer
	CommonTags           util.ELBv2Tags
	Logger               *log.Logger
}

// NewDesiredTargetGroups returns a new targetgroups.TargetGroups based on an extensions.Ingress.
func NewDesiredTargetGroups(o *NewDesiredTargetGroupsOptions) (TargetGroups, error) {
	output := o.ExistingTargetGroups

	annos, err := o.Store.GetIngressAnnotations(k8s.MetaNamespaceKey(o.Ingress))
	if err != nil {
		return nil, err
	}

	for _, rule := range o.Ingress.Spec.Rules {
		for _, path := range rule.HTTP.Paths {

			serviceKey := fmt.Sprintf("%s/%s", o.Ingress.Namespace, path.Backend.ServiceName)

			tgAnnotations, err := o.Store.GetServiceAnnotations(serviceKey, annos)
			if err != nil {
				return nil, fmt.Errorf(fmt.Sprintf("Error getting Service annotations, %v", err.Error()))
			}

			targetPort, err := o.Store.GetServicePort(path.Backend, o.Ingress.Namespace, *tgAnnotations.TargetGroup.TargetType)
			if err != nil {
				return nil, err
			}

			targets, err := o.Store.GetTargets(tgAnnotations.TargetGroup.TargetType, o.Ingress.Namespace, path.Backend.ServiceName, targetPort)
			if err != nil {
				return nil, err
			}

			if *tgAnnotations.TargetGroup.TargetType == "pod" {
				err := targets.PopulateAZ()
				if err != nil {
					return nil, err
				}
			}

			// Start with a new target group with a new Desired state.
			targetGroup := NewDesiredTargetGroup(&NewDesiredTargetGroupOptions{
				Ingress:        o.Ingress,
				Annotations:    tgAnnotations,
				CommonTags:     o.CommonTags,
				Store:          o.Store,
				LoadBalancerID: o.LoadBalancerID,
				TargetPort:     targetPort,
				Logger:         o.Logger,
				SvcName:        path.Backend.ServiceName,
				SvcPort:        path.Backend.ServicePort,
				Targets:        targets,
			})

			// If this target group is already defined, copy the current state to our new TG
			if i, _ := o.ExistingTargetGroups.FindById(targetGroup.ID); i >= 0 {
				output[i].copyDesiredState(targetGroup)
			} else {
				output = append(output, targetGroup)
			}
		}
	}
	return output, nil
}
