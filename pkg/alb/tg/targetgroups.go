package tg

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

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
		targetgroup.tags.desired = nil
		targetgroup.tg.desired = nil
		targetgroup.targets.desired = nil
	}
}

type NewCurrentTargetGroupsOptions struct {
	TargetGroups   []*elbv2.TargetGroup
	ResourceTags   *albrgt.Resources
	ALBNamePrefix  string
	LoadBalancerID string
	Logger         *log.Logger
}

// NewCurrentTargetGroups returns a new targetgroups.TargetGroups based on an elbv2.TargetGroups.
func NewCurrentTargetGroups(o *NewCurrentTargetGroupsOptions) (TargetGroups, error) {
	var output TargetGroups

	for _, targetGroup := range o.TargetGroups {
		tg, err := NewCurrentTargetGroup(&NewCurrentTargetGroupOptions{
			TargetGroup:    targetGroup,
			ResourceTags:   o.ResourceTags,
			ALBNamePrefix:  o.ALBNamePrefix,
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
	IngressRules          []extensions.IngressRule
	LoadBalancerID        string
	ExistingTargetGroups  TargetGroups
	AnnotationFactory     annotations.AnnotationFactory
	IngressAnnotations    *map[string]string
	ALBNamePrefix         string
	Namespace             string
	CommonTags            util.ELBv2Tags
	Logger                *log.Logger
	GetServiceNodePort    func(string, int32) (*int64, error)
	GetServiceAnnotations func(string, string) (*map[string]string, error)
	TargetsFunc           func(*string, string, string, *int64) albelbv2.TargetDescriptions
}

// NewDesiredTargetGroups returns a new targetgroups.TargetGroups based on an extensions.Ingress.
func NewDesiredTargetGroups(o *NewDesiredTargetGroupsOptions) (TargetGroups, error) {
	var output TargetGroups

	for _, rule := range o.IngressRules {
		for _, path := range rule.HTTP.Paths {

			serviceKey := fmt.Sprintf("%s/%s", o.Namespace, path.Backend.ServiceName)
			port, err := o.GetServiceNodePort(serviceKey, path.Backend.ServicePort.IntVal)
			if err != nil {
				return nil, err
			}

			tgAnnotations, err := mergeAnnotations(&mergeAnnotationsOptions{
				AnnotationFactory:     o.AnnotationFactory,
				IngressAnnotations:    o.IngressAnnotations,
				Namespace:             o.Namespace,
				ServiceName:           path.Backend.ServiceName,
				GetServiceAnnotations: o.GetServiceAnnotations,
			})
			if err != nil {
				return output, err
			}

			// Start with a new target group with a new Desired state.
			targetGroup := NewDesiredTargetGroup(&NewDesiredTargetGroupOptions{
				Annotations:    tgAnnotations,
				CommonTags:     o.CommonTags,
				ALBNamePrefix:  o.ALBNamePrefix,
				LoadBalancerID: o.LoadBalancerID,
				Port:           *port,
				Logger:         o.Logger,
				Namespace:      o.Namespace,
				SvcName:        path.Backend.ServiceName,
				Targets:        o.TargetsFunc(tgAnnotations.RoutingTarget, o.Namespace, path.Backend.ServiceName, port),
			})

			// If this target group is already defined, copy the current state to our new TG
			if i, tg := o.ExistingTargetGroups.FindById(targetGroup.ID); i >= 0 {
				targetGroup.tg.current = tg.tg.current
				targetGroup.attributes.current = tg.attributes.current
				targetGroup.targets.current = tg.targets.current
				targetGroup.tags.current = tg.tags.current

				// If there is a current TG ARN we can use it to purge the desired targets of unready instances
				if tg.CurrentARN() != nil {
					desired, err := albelbv2.ELBV2svc.DescribeTargetGroupTargetsForArn(tg.CurrentARN(), targetGroup.targets.desired)
					if err != nil {
						return nil, err
					}
					targetGroup.targets.desired = desired
				}
			}

			output = append(output, targetGroup)
		}
	}
	return output, nil
}

type mergeAnnotationsOptions struct {
	AnnotationFactory     annotations.AnnotationFactory
	IngressAnnotations    *map[string]string
	Namespace             string
	ServiceName           string
	GetServiceAnnotations func(string, string) (*map[string]string, error)
}

func mergeAnnotations(o *mergeAnnotationsOptions) (*annotations.Annotations, error) {
	serviceAnnotations, err := o.GetServiceAnnotations(o.Namespace, o.ServiceName)
	if err != nil {
		return nil, err
	}

	mergedAnnotations := make(map[string]string)
	for k, v := range *o.IngressAnnotations {
		mergedAnnotations[k] = v
	}

	for k, v := range *serviceAnnotations {
		mergedAnnotations[k] = v
	}

	tgAnnotations, err := o.AnnotationFactory.ParseAnnotations(&annotations.ParseAnnotationsOptions{
		Annotations: mergedAnnotations,
		Namespace:   o.Namespace,
		ServiceName: o.ServiceName,
	})

	if err != nil {
		msg := fmt.Errorf("Error parsing service annotations: %s", err.Error())
		return nil, msg
	}
	return tgAnnotations, nil
}
