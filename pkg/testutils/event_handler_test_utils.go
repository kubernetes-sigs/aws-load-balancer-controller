package testutils

import (
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
)

func ExtractCTRLRequestsFromQueue(queue workqueue.RateLimitingInterface) []ctrl.Request {
	var requests []ctrl.Request
	for queue.Len() > 0 {
		item, _ := queue.Get()
		queue.Done(item)
		requests = append(requests, item.(ctrl.Request))
	}
	return requests
}
