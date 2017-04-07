package alb

import (
	"github.com/coreos/alb-ingress-controller/log"
)

type TargetGroups []*TargetGroup

func (t TargetGroups) LookupBySvc(svc string) int {
	for p, v := range t {
		if v.SvcName == svc {
			return p
		}
	}
	log.Infof("No TG matching service found. SVC %s", "controller", svc)
	return -1
}

func (t TargetGroups) Find(tg *TargetGroup) int {
	for p, v := range t {
		if *v.Id == *tg.Id {
			return p
		}
	}
	return -1
}

// SyncState kicks off the state synchronization for every target group inside this TargetGroups
// instance.
func (t TargetGroups) SyncState(lb *LoadBalancer) TargetGroups {
	var targetgroups TargetGroups
	for _, targetgroup := range t {
		tg := targetgroup.SyncState(lb)
		if tg != nil {
			targetgroups = append(targetgroups, tg)
		}
	}
	return targetgroups
}

func (t TargetGroups) StripDesiredState() {
	for _, targetgroup := range t {
		targetgroup.DesiredTags = nil
		targetgroup.DesiredTargetGroup = nil
		targetgroup.DesiredTargets = nil
	}
}
