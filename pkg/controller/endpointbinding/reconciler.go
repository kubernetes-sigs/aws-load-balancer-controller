package endpointbinding

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
)

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, cloud cloud.Cloud, ebRepo backend.EndpointBindingRepo, cache cache.Cache) reconcile.Reconciler {
	endpointResolver := backend.NewEndpointResolver(cloud, cache)
	return &ReconcileEndpointBinding{
		cloud:            cloud,
		ebRepo:           ebRepo,
		endpointResolver: endpointResolver,
		cache:            cache,
	}
}

var _ reconcile.Reconciler = &ReconcileEndpointBinding{}

// ReconcileEndpointBinding reconciles a EndpointBinding object
type ReconcileEndpointBinding struct {
	cloud            cloud.Cloud
	ebRepo           backend.EndpointBindingRepo
	endpointResolver backend.EndpointResolver
	cache            cache.Cache
}

func (r *ReconcileEndpointBinding) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	eb, err := r.ebRepo.Get(ctx, request.NamespacedName)
	if err != nil {
		if k8serr.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	svcKey := types.NamespacedName{
		Namespace: eb.Spec.ServiceRef.Namespace,
		Name:      eb.Spec.ServiceRef.Name,
	}

	desiredTargets, err := r.endpointResolver.Resolve(ctx, svcKey, eb.Spec.ServicePort, eb.Spec.TargetType)
	if err != nil {
		return reconcile.Result{}, err
	}

	tgArn := eb.Spec.TargetGroup.TargetGroupARN
	currentTargets, err := r.getCurrentTargets(ctx, tgArn)
	if err != nil {
		return reconcile.Result{}, err
	}

	additions, removals := r.computeTargetsChangeSet(currentTargets, desiredTargets)
	if len(additions) > 0 {
		in := &elbv2.RegisterTargetsInput{
			TargetGroupArn: aws.String(tgArn),
			Targets:        additions,
		}

		logging.FromContext(ctx).Info("adding targets", "tgArn", tgArn, "changes", tdsString(additions))
		if _, err := r.cloud.ELBV2().RegisterTargetsWithContext(ctx, in); err != nil {
			return reconcile.Result{}, err
		}
		logging.FromContext(ctx).Info("added targets", "tgArn", tgArn)
	}

	if len(removals) > 0 {
		in := &elbv2.DeregisterTargetsInput{
			TargetGroupArn: aws.String(tgArn),
			Targets:        removals,
		}

		logging.FromContext(ctx).Info("removing targets", "tgArn", tgArn, "changes", tdsString(removals))
		if _, err := r.cloud.ELBV2().DeregisterTargetsWithContext(ctx, in); err != nil {
			return reconcile.Result{}, err
		}
		logging.FromContext(ctx).Info("removed targets", "tgArn", tgArn)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileEndpointBinding) getCurrentTargets(ctx context.Context, tgArn string) ([]*elbv2.TargetDescription, error) {
	resp, err := r.cloud.ELBV2().DescribeTargetHealthWithContext(ctx, &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgArn),
	})
	if err != nil {
		return nil, err
	}

	var current []*elbv2.TargetDescription
	for _, thd := range resp.TargetHealthDescriptions {
		if aws.StringValue(thd.TargetHealth.State) == elbv2.TargetHealthStateEnumDraining {
			continue
		}
		current = append(current, thd.Target)
	}
	return current, nil
}

// computeTargetsChangeSet compares desired to current, returning a list of targets to add and remove from current to match desired
func (r *ReconcileEndpointBinding) computeTargetsChangeSet(current []*elbv2.TargetDescription, desired []*elbv2.TargetDescription) (add []*elbv2.TargetDescription, remove []*elbv2.TargetDescription) {
	currentMap := map[string]bool{}
	desiredMap := map[string]bool{}

	for _, i := range current {
		currentMap[tdString(i)] = true
	}
	for _, i := range desired {
		desiredMap[tdString(i)] = true
	}

	for _, i := range desired {
		if _, ok := currentMap[tdString(i)]; !ok {
			add = append(add, i)
		}
	}

	for _, i := range current {
		if _, ok := desiredMap[tdString(i)]; !ok {
			remove = append(remove, i)
		}
	}

	return add, remove
}

func tdString(td *elbv2.TargetDescription) string {
	return fmt.Sprintf("%v:%v", aws.StringValue(td.Id), aws.Int64Value(td.Port))
}

func tdsString(tds []*elbv2.TargetDescription) string {
	var s []string
	for _, td := range tds {
		s = append(s, tdString(td))
	}
	return strings.Join(s, ", ")
}
