package tg

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"

	"github.com/aws/aws-sdk-go/service/elbv2"
	extensions "k8s.io/api/extensions/v1beta1"
)

// LookupByBackend returns the position of a TargetGroup by an IngressBackend, returning -1 if unfound.
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
	Metric         metric.Collector
}

// NewCurrentTargetGroups returns a new targetgroups.TargetGroups based on an elbv2.TargetGroups.
func NewCurrentTargetGroups(o *NewCurrentTargetGroupsOptions) (TargetGroups, error) {
	var output TargetGroups

	for _, targetGroup := range o.TargetGroups {
		tg, err := NewCurrentTargetGroup(&NewCurrentTargetGroupOptions{
			TargetGroup:    targetGroup,
			LoadBalancerID: o.LoadBalancerID,
			Logger:         o.Logger,
			Metric:         o.Metric,
		})
		if err != nil {
			return nil, err
		}
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
	Metric               metric.Collector
}

// NewDesiredTargetGroups returns a new targetgroups.TargetGroups based on an extensions.Ingress.
func NewDesiredTargetGroups(o *NewDesiredTargetGroupsOptions) (TargetGroups, error) {
	var output TargetGroups
	var backends []*extensions.IngressBackend

	if o.Ingress.Spec.Backend != nil {
		backends = append(backends, o.Ingress.Spec.Backend)
	}

	for _, rule := range o.Ingress.Spec.Rules {
		for i := range rule.HTTP.Paths {
			backends = append(backends, &rule.HTTP.Paths[i].Backend)
		}
	}

	for _, backend := range backends {
		if backend.ServicePort.String() == "use-annotation" {
			// action annotations do not need target groups
			continue
		}

		targetGroup, err := NewDesiredTargetGroupFromBackend(&NewDesiredTargetGroupFromBackendOptions{
			Backend:              backend,
			CommonTags:           o.CommonTags,
			LoadBalancerID:       o.LoadBalancerID,
			Store:                o.Store,
			Ingress:              o.Ingress,
			Logger:               o.Logger,
			ExistingTargetGroups: o.ExistingTargetGroups,
			Metric:               o.Metric,
		})

		if err != nil {
			return nil, err
		}

		output = append(output, targetGroup)
	}

	return output, nil
}
