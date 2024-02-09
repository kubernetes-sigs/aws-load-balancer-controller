package fixture

import (
	"context"
	"github.com/pkg/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sort"
)

var (
	defaultKindOrdering = []string{
		"IngressClass",
		"Namespace",
		"Service",
		"Deployment",
		"Ingress",
	}
)

// NewK8SResourceStack constructs new Resource stack.
func NewK8SResourceStack(tf *framework.Framework, objs ...client.Object) *k8sResourceStack {
	return &k8sResourceStack{
		tf:              tf,
		objs:            objs,
		provisionedObjs: nil,
		kindOrdering:    defaultKindOrdering,
	}
}

type k8sResourceStack struct {
	tf              *framework.Framework
	objs            []client.Object
	provisionedObjs []client.Object
	kindOrdering    []string
}

func (s *k8sResourceStack) Setup(ctx context.Context) error {
	scheme := s.tf.K8sClient.Scheme()
	sortedObjs, err := s.sortedObjectsOrderingByKind(s.kindOrdering)
	if err != nil {
		return err
	}
	for _, obj := range sortedObjs {
		objKind, _ := apiutil.GVKForObject(obj, scheme)
		s.tf.Logger.Info("creating resource", "kind", objKind.Kind, "key", k8s.NamespacedName(obj))
		if err := s.tf.K8sClient.Create(ctx, obj); err != nil {
			return err
		}
		s.provisionedObjs = append(s.provisionedObjs, obj)
		s.tf.Logger.Info("created resource", "kind", objKind.Kind, "key", k8s.NamespacedName(obj))
	}
	return nil
}

func (s *k8sResourceStack) TearDown(ctx context.Context) error {
	scheme := s.tf.K8sClient.Scheme()
	for i := len(s.provisionedObjs) - 1; i >= 0; i-- {
		obj := s.provisionedObjs[i]
		objKind, _ := apiutil.GVKForObject(obj, scheme)
		s.tf.Logger.Info("deleting resource", "kind", objKind.Kind, "key", k8s.NamespacedName(obj))
		if err := s.tf.K8sClient.Delete(ctx, obj); err != nil {
			return err
		}
		s.tf.Logger.Info("deleted resource", "kind", objKind.Kind, "key", k8s.NamespacedName(obj))

		s.tf.Logger.Info("wait resource becomes deleted", "kind", objKind.Kind, "key", k8s.NamespacedName(obj))
		if err := s.waitUntilResourceDeleted(ctx, obj); err != nil {
			return err
		}
		s.tf.Logger.Info("resource becomes deleted", "kind", objKind.Kind, "key", k8s.NamespacedName(obj))
	}
	return nil
}

type objWithOrder struct {
	obj   client.Object
	order int
}

func (s *k8sResourceStack) sortedObjectsOrderingByKind(kindOrdering []string) ([]client.Object, error) {
	ordering := make(map[string]int, len(kindOrdering))
	for i, k := range kindOrdering {
		ordering[k] = i
	}

	scheme := s.tf.K8sClient.Scheme()
	var objWithOrderList []objWithOrder
	for _, obj := range s.objs {
		objKind, err := apiutil.GVKForObject(obj, scheme)
		if err != nil {
			return nil, err
		}
		order, ok := ordering[objKind.Kind]
		if !ok {
			return nil, errors.Errorf("ordering for %v Kind is not configured", objKind)
		}
		objWithOrderList = append(objWithOrderList, objWithOrder{
			obj:   obj,
			order: order,
		})
	}
	sort.SliceStable(objWithOrderList, func(i, j int) bool {
		return objWithOrderList[i].order < objWithOrderList[j].order
	})

	var sortedObjs []client.Object
	for _, objWithOrder := range objWithOrderList {
		sortedObjs = append(sortedObjs, objWithOrder.obj)
	}
	return sortedObjs, nil
}

func (s *k8sResourceStack) waitUntilResourceDeleted(ctx context.Context, obj client.Object) error {
	observedObj := obj.DeepCopyObject().(client.Object)
	return wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := s.tf.K8sClient.Get(ctx, k8s.NamespacedName(obj), observedObj); err != nil {
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, ctx.Done())
}
