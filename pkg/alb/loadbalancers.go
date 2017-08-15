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

// Reconcile calls for state synchronization (comparison of current and desired) for the load
// balancer and its resource record set, target group(s), and listener(s). It returns 2
// LoadBalancers (slices), the first being the list of all known LoadBalancers and the subset
// second being of LoadBalancers, from the first list, that failed to reconcile.
func (l LoadBalancers) Reconcile(rOpts *ReconcileOptions) (LoadBalancers, LoadBalancers) {
	loadbalancers := l
	errLBs := LoadBalancers{}

	for i, loadbalancer := range l {
		rOpts.loadbalancer = loadbalancer

		if err := loadbalancer.Reconcile(rOpts); err != nil {
			loadbalancer.LastError = err
			errLBs = append(errLBs, loadbalancer)
			continue
		}
		if err := loadbalancer.TargetGroups.Reconcile(rOpts); err != nil {
			loadbalancer.LastError = err
			errLBs = append(errLBs, loadbalancer)
			continue
		}
		// This syncs listeners and rules
		if err := loadbalancer.Listeners.Reconcile(rOpts); err != nil {
			loadbalancer.LastError = err
			errLBs = append(errLBs, loadbalancer)
			continue
		}
		// If the lb was deleted, remove it from the list to be returned.
		if loadbalancer.Deleted {
			loadbalancers = append(loadbalancers[:i], loadbalancers[i+1:]...)
		}
	}

	return loadbalancers, errLBs
}

// StripDesiredState removes the DesiredLoadBalancers from a LoadBalancers slice
func (l LoadBalancers) StripDesiredState() {
	for _, lb := range l {
		lb.DesiredLoadBalancer = nil
	}
}
