package util

import (
	"sort"

	"k8s.io/apimachinery/pkg/types"
)

type ReconcileCounters struct {
	serviceReconciles map[types.NamespacedName]int
	ingressReconciles map[types.NamespacedName]int
	tgbReconciles     map[types.NamespacedName]int
}

type ResourceReconcileCount struct {
	Resource types.NamespacedName
	Count    int
}

func NewReconcileCounters() *ReconcileCounters {
	return &ReconcileCounters{
		serviceReconciles: make(map[types.NamespacedName]int),
		ingressReconciles: make(map[types.NamespacedName]int),
		tgbReconciles:     make(map[types.NamespacedName]int),
	}
}

func (c *ReconcileCounters) IncrementService(namespaceName types.NamespacedName) {
	c.serviceReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementIngress(namespaceName types.NamespacedName) {
	c.ingressReconciles[namespaceName]++
}

func (c *ReconcileCounters) IncrementTGB(namespaceName types.NamespacedName) {
	c.tgbReconciles[namespaceName]++
}

func (c *ReconcileCounters) GetTopReconciles(n int) map[string][]ResourceReconcileCount {
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

	return topReconciles
}
