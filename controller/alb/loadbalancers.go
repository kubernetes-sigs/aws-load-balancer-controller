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
func (l LoadBalancers) SyncState() (LoadBalancers, error) {
	loadbalancers := l

	for i, loadbalancer := range l {

		if err := loadbalancer.SyncState(); err != nil {
			return loadbalancers, err
		}
		if err := loadbalancer.ResourceRecordSet.SyncState(loadbalancer); err != nil {
			return loadbalancers, err
		}
		if err := loadbalancer.TargetGroups.SyncState(loadbalancer); err != nil {
			return loadbalancers, err
		}
		// This syncs listeners and rules
		if err := loadbalancer.Listeners.SyncState(loadbalancer, &loadbalancer.TargetGroups); err != nil {
			return loadbalancers, err
		}
		// If the lb was deleted, remove it from the list to be returned.
		if loadbalancer.Deleted {
			loadbalancers = append(loadbalancers[:i], loadbalancers[i+1:]...)
		}
	}

	return loadbalancers, nil
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
