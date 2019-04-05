package backend

import (
	"context"
	"fmt"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EndpointBindingRepo interface {
	Create(ctx context.Context, obj *api.EndpointBinding) error
	Update(ctx context.Context, obj *api.EndpointBinding) error
	Delete(ctx context.Context, obj *api.EndpointBinding) error

	Get(ctx context.Context, key types.NamespacedName) (*api.EndpointBinding, error)
	List(ctx context.Context, options *client.ListOptions) (*api.EndpointBindingList, error)
	Watch(ctx context.Context) (watch.Interface, error)
}

type IndexFunc func(binding *api.EndpointBinding) []string

func NewEndpointBindingRepo(fieldIndexers map[string]IndexFunc) EndpointBindingRepo {
	indexFuncs := make(map[string]cache.IndexFunc)
	for field, _ := range fieldIndexers {
		fieldIndexer := fieldIndexers[field]
		indexFunc := func(obj interface{}) ([]string, error) {
			eb := obj.(*api.EndpointBinding)
			return fieldIndexer(eb), nil
		}
		indexFuncs[fieldIndexName(field)] = indexFunc
	}

	keyFunc := func(obj interface{}) (s string, e error) {
		eb := obj.(*api.EndpointBinding)
		return objectKeyToStoreKey(k8s.NamespacedName(eb)), nil
	}
	indexer := cache.NewIndexer(keyFunc, indexFuncs)
	broadcaster := watch.NewBroadcaster(100, watch.WaitIfChannelFull)

	return &defaultEndpointBindingRepo{
		indexer:     indexer,
		broadcaster: broadcaster,
	}
}

var _ EndpointBindingRepo = (*defaultEndpointBindingRepo)(nil)

type defaultEndpointBindingRepo struct {
	indexer     cache.Indexer
	broadcaster *watch.Broadcaster
}

func (r *defaultEndpointBindingRepo) Create(ctx context.Context, obj *api.EndpointBinding) error {
	if err := r.indexer.Add(obj); err != nil {
		return err
	}
	r.broadcaster.Action(watch.Added, obj)
	return nil
}

func (r *defaultEndpointBindingRepo) Update(ctx context.Context, obj *api.EndpointBinding) error {
	if err := r.indexer.Update(obj); err != nil {
		return err
	}
	r.broadcaster.Action(watch.Modified, obj)
	return nil
}

func (r *defaultEndpointBindingRepo) Delete(ctx context.Context, obj *api.EndpointBinding) error {
	if err := r.indexer.Delete(obj); err != nil {
		return err
	}
	r.broadcaster.Action(watch.Deleted, obj)
	return nil
}

func (r *defaultEndpointBindingRepo) Get(ctx context.Context, key types.NamespacedName) (*api.EndpointBinding, error) {
	storeKey := objectKeyToStoreKey(key)
	obj, exists, err := r.indexer.GetByKey(storeKey)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, k8serr.NewNotFound(api.Resource("EndpointBinding"), key.Name)
	}
	return obj.(*api.EndpointBinding).DeepCopy(), nil
}

func (r *defaultEndpointBindingRepo) List(ctx context.Context, options *client.ListOptions) (*api.EndpointBindingList, error) {
	var objs []interface{}
	var err error
	if options != nil && options.FieldSelector != nil {
		field, val, requires := requiresExactMatch(options.FieldSelector)
		if !requires {
			return nil, fmt.Errorf("non-exact field matches are not supported")
		}
		objs, err = r.indexer.ByIndex(fieldIndexName(field), val)
	} else {
		objs = r.indexer.List()
	}
	if err != nil {
		return nil, err
	}
	runtimeObjs := make([]runtime.Object, 0, len(objs))
	for _, item := range objs {
		obj, isObj := item.(runtime.Object)
		if !isObj {
			return nil, fmt.Errorf("cache contained %T, which is not an Object", obj)
		}
		runtimeObjs = append(runtimeObjs, obj)
	}
	ebList := &api.EndpointBindingList{}
	if err := apimeta.SetList(ebList, runtimeObjs); err != nil {
		return nil, err
	}
	return ebList, nil
}

func (r *defaultEndpointBindingRepo) Watch(ctx context.Context) (watch.Interface, error) {
	return r.broadcaster.Watch(), nil
}

func objectKeyToStoreKey(k types.NamespacedName) string {
	if k.Namespace == "" {
		return k.Name
	}
	return k.Namespace + "/" + k.Name
}

func requiresExactMatch(sel fields.Selector) (field, value string, required bool) {
	reqs := sel.Requirements()
	if len(reqs) != 1 {
		return "", "", false
	}
	req := reqs[0]
	if req.Operator != selection.Equals && req.Operator != selection.DoubleEquals {
		return "", "", false
	}

	return req.Field, req.Value, true
}

func fieldIndexName(field string) string {
	return "field:" + field
}
