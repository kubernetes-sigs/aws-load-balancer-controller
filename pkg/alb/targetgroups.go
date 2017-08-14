package alb

// TargetGroups is a slice of TargetGroup pointers
type TargetGroups []*TargetGroup

// LookupBySvc returns the position of a TargetGroup by its SvcName, returning -1 if unfound.
func (t TargetGroups) LookupBySvc(svc string) int {
	for p, v := range t {
		if v.SvcName == svc {
			return p
		}
	}
	// LOG: log.Infof("No TG matching service found. SVC %s", "controller", svc)
	return -1
}

// Find returns the position of a TargetGroup by its ID, returning -1 if unfound.
func (t TargetGroups) Find(tg *TargetGroup) int {
	for p, v := range t {
		if *v.ID == *tg.ID {
			return p
		}
	}
	return -1
}

// Reconcile kicks off the state synchronization for every target group inside this TargetGroups
// instance.
func (t TargetGroups) Reconcile(rOpts *ReconcileOptions) error {
	lb := rOpts.loadbalancer
	for _, targetgroup := range t {
		if err := targetgroup.Reconcile(rOpts); err != nil {
			return err
		}
		if targetgroup.deleted {
			i := lb.TargetGroups.Find(targetgroup)
			lb.TargetGroups = append(lb.TargetGroups[:i], lb.TargetGroups[i+1:]...)
		}
	}

	return nil
}

// StripDesiredState removes the DesiredTags, DesiredTargetGroup, and DesiredTargets from all TargetGroups
func (t TargetGroups) StripDesiredState() {
	for _, targetgroup := range t {
		targetgroup.DesiredTags = nil
		targetgroup.DesiredTargetGroup = nil
		targetgroup.DesiredTargets = nil
	}
}
