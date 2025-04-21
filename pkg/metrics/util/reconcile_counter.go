package util

import (
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type ReconcileCounters struct {
	serviceReconciles    map[types.NamespacedName]int
	ingressReconciles    map[types.NamespacedName]int
	tgbReconciles        map[types.NamespacedName]int
	nlbGatewayReconciles map[types.NamespacedName]int
	albGatewayReconciles map[types.NamespacedName]int
	mutex                sync.Mutex
}

type ResourceReconcileCount struct {
	Resource types.NamespacedName
	Count    int
}

func NewReconcileCounters() *ReconcileCounters {
	return &ReconcileCounters{
		serviceReconciles:    make(map[types.NamespacedName]int),
		ingressReconciles:    make(map[types.NamespacedName]int),
		tgbReconciles:        make(map[types.NamespacedName]int),
		albGatewayReconciles: make(map[types.NamespacedName]int),
		nlbGatewayReconciles: make(map[types.NamespacedName]int),
		mutex:                sync.Mutex{},
	}
}

func (c *ReconcileCounters) IncrementService(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.serviceReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementIngress(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.ingressReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementTGB(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.tgbReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementNLBGateway(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.nlbGatewayReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementALBGateway(namespaceName types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.albGatewayReconciles[namespaceName]++
}
func (c *ReconcileCounters) GetTopReconciles(n int) map[string][]ResourceReconcileCount {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	topReconciles := make(map[string][]ResourceReconcileCount)
	getTopN := func(m map[types.NamespacedName]int) []ResourceReconcileCount {
		reconciles := make([]ResourceReconcileCount, 0, len(m))
		for k, v := range m {
			reconciles = append(reconciles, ResourceReconcileCount{Resource: k, Count: v})
		}

		sort.Slice(reconciles, func(i, j int) bool {
			return reconciles[i].Count > reconciles[j].Count
		})
		if len(reconciles) > n {
			reconciles = reconciles[:n]
		}
		return reconciles
	}

	topReconciles["service"] = getTopN(c.serviceReconciles)
	topReconciles["ingress"] = getTopN(c.ingressReconciles)
	topReconciles["targetgroupbinding"] = getTopN(c.tgbReconciles)
	topReconciles["nlbgateway"] = getTopN(c.nlbGatewayReconciles)
	topReconciles["albgateway"] = getTopN(c.albGatewayReconciles)

	return topReconciles
}

func (c *ReconcileCounters) ResetCounter() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.serviceReconciles = make(map[types.NamespacedName]int)
	c.ingressReconciles = make(map[types.NamespacedName]int)
	c.tgbReconciles = make(map[types.NamespacedName]int)
	c.nlbGatewayReconciles = make(map[types.NamespacedName]int)
	c.albGatewayReconciles = make(map[types.NamespacedName]int)
}
