package k8s

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type TypedInformer[T any] struct {
	Informer cache.Informer
	Handler  handler.TypedEventHandler[T, reconcile.Request]
}

var _ source.Source = &TypedInformer[string]{}

func (is *TypedInformer[T]) Start(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	if is.Informer == nil {
		return fmt.Errorf("must specify Informer.Informer")
	}
	if is.Handler == nil {
		return errors.New("must specify Informer.Handler")
	}

	handler := toolscache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, isInInitialList bool) {
			is.Handler.Create(ctx, event.TypedCreateEvent[T]{
				IsInInitialList: isInInitialList,
				Object:          obj.(T),
			}, queue)
		},
		UpdateFunc: func(oldObj, newObj any) {
			is.Handler.Update(ctx, event.TypedUpdateEvent[T]{
				ObjectOld: oldObj.(T),
				ObjectNew: newObj.(T),
			}, queue)
		},
		DeleteFunc: func(obj any) {
			typedObj, ok := typedInformerDeletedObject[T](obj)
			if !ok {
				utilruntime.HandleErrorWithContext(ctx, nil, "typedInformer: unexpected object type in delete event", "object", fmt.Sprintf("%T", obj))
				return
			}
			is.Handler.Delete(ctx, event.TypedDeleteEvent[T]{
				Object: typedObj,
			}, queue)
		},
	}

	_, err := is.Informer.AddEventHandlerWithOptions(handler, toolscache.HandlerOptions{})
	return err
}

// typedInformerDeletedObject unwraps a cache.DeletedFinalStateUnknown tombstone
// (delivered when the informer misses a delete event, e.g. after a watch relist)
// before asserting T, so a missed delete no longer panics the whole controller.
func typedInformerDeletedObject[T any](obj any) (T, bool) {
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	typedObj, ok := obj.(T)
	return typedObj, ok
}
