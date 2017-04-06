package controller

import (
	"github.com/coreos-inc/alb-ingress-controller/pkg/cmd/log"
)

type TargetGroups []*TargetGroup

func (t TargetGroups) LookupBySvc(svc string) int {
	for p, v := range t {
		if v.SvcName == svc {
			log.Infof("Search for a TG matching a service was successful. SVC: %s | TG: %s", "controller", svc, log.Prettify(v))
			return p
		}
	}
	log.Infof("No TG matching service found. SVC %s", "controller", svc)
	return -1
}

func (t TargetGroups) find(tg *TargetGroup) int {
	for p, v := range t {
		if *v.id == *tg.id {
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

// func (t TargetGroups) modify(a *ALBIngress, lb *LoadBalancer) error {
// 	var tg TargetGroups

// 	for _, targetGroup := range lb.TargetGroups {
// 		if targetGroup.DesiredTargetGroup == nil {
// 			lb.Listeners = lb.Listeners.purgeTargetGroupArn(a, targetGroup.CurrentTargetGroup.TargetGroupArn)
// 			targetGroup.delete()
// 			continue
// 		}

// 		if targetGroup.CurrentTargetGroup == nil {
// 			targetGroup.create(a, lb)
// 			continue
// 		}

// 		if targetGroup.needsModification() {
// 			err := targetGroup.modify(a, lb)
// 			if err != nil {
// 				glog.Errorf("%s: Error when modifying target group %s: %s", a.Name(), *targetGroup.id, err)
// 				tg = append(tg, targetGroup)
// 				continue
// 			}

// 			for {
// 				glog.Infof("%s: Waiting for target group %s to be online", a.Name(), *targetGroup.id)
// 				if targetGroup.online(a) == true {
// 					break
// 				}
// 				time.Sleep(5 * time.Second)
// 			}
// 		}

// 		tg = append(tg, targetGroup)
// 	}

// 	lb.TargetGroups = tg
// 	return nil
// }

// func (t TargetGroups) delete() error {
// 	errors := false
// 	for _, targetGroup := range t {
// 		if err := targetGroup.delete(); err != nil {
// 			glog.Infof("Unable to delete target group %s: %s",
// 				*targetGroup.id,
// 				err)
// 			errors = true
// 		}
// 	}
// 	if errors {
// 		return fmt.Errorf("There were errors deleting target groups")
// 	}
// 	return nil
// }

func (t TargetGroups) StripDesiredState() {
	for _, targetgroup := range t {
		targetgroup.DesiredTags = nil
		targetgroup.DesiredTargetGroup = nil
		targetgroup.DesiredTargets = nil
	}
}
