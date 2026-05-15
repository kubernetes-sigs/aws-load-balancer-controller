package k8s

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
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
			is.Handler.Delete(ctx, event.TypedDeleteEvent[T]{
				Object: obj.(T),
			}, queue)
		},
	}

	_, err := is.Informer.AddEventHandlerWithOptions(handler, toolscache.HandlerOptions{})
	return err
}
