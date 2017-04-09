package alb

// LoadBalancers is a slice of LoadBalancer pointers
type LoadBalancers []*LoadBalancer

// Find returns the position of the lb parameter within the LoadBalancers slice, -1 if it is not found
func (l LoadBalancers) Find(lb *LoadBalancer) int {
	for i, lbi := range l {
		if *lb.ID == *lbi.ID {
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

// StripDesiredState removes the DesiredLoadBalancers from a LoadBalancers slice
func (l LoadBalancers) StripDesiredState() {
	for _, lb := range l {
		lb.DesiredLoadBalancer = nil
		if lb.ResourceRecordSet != nil {
			lb.ResourceRecordSet.DesiredResourceRecordSet = nil
		}
	}
}
