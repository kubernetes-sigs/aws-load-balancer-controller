package controller

import (
	"github.com/golang/glog"
)

type LoadBalancers []*LoadBalancer

func (l LoadBalancers) find(lb *LoadBalancer) int {
	for i, lbi := range l {
		if *lb.id == *lbi.id {
			return i
		}
	}
	return -1
}

// SyncState calls for state synchronization (comparison of current and desired) for the load
// balancer and its resource record set, target group(s), and listener(s).
func (l LoadBalancers) SyncState() LoadBalancers {
	var loadbalancers LoadBalancers

	for _, loadbalancer := range l {
		lb := loadbalancer.SyncState()
		loadbalancer.ResourceRecordSet = loadbalancer.ResourceRecordSet.SyncState(loadbalancer)
		loadbalancer.TargetGroups = loadbalancer.TargetGroups.SyncState(loadbalancer)
		loadbalancer.Listeners = loadbalancer.Listeners.SyncState(lb, &loadbalancer.TargetGroups)
		// Only add lb's back to the list that are non-nil and weren't fully deleted.
		if lb != nil && !lb.Deleted {
			loadbalancers = append(loadbalancers, lb)
		}
	}
	return loadbalancers
}

func (l LoadBalancers) StripDesiredState() {
	for _, lb := range l {
		lb.DesiredLoadBalancer = nil
		if lb.ResourceRecordSet != nil {
			lb.ResourceRecordSet.DesiredResourceRecordSet = nil
		}
	}
}
