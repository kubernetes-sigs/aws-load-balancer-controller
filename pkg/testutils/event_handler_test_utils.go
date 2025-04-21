package testutils

import (
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func ExtractCTRLRequestsFromQueue(queue workqueue.TypedRateLimitingInterface[reconcile.Request]) []reconcile.Request {
	var requests []reconcile.Request
	for queue.Len() > 0 {
		item, _ := queue.Get()
		queue.Done(item)
		requests = append(requests, item)
	}
	return requests
}
