package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
)

type TargetGroups []*TargetGroup

func (t TargetGroups) find(tg *TargetGroup) int {
	for p, v := range t {
		if *v.id == *tg.id {
			return p
		}
	}
	return -1
}

func (t TargetGroups) modify(a *albIngress, lb *LoadBalancer) error {
	var tg TargetGroups

	for _, targetGroup := range lb.TargetGroups {
		if targetGroup.needsModification() {
			err := targetGroup.modify(a, lb)
			if err != nil {
				glog.Errof("%s: Error when modifying target group %s: %s", a.Name(), *targetGroup.id, err)
				tg = append(tg, targetGroup)
				continue
			}

			for {
				glog.Infof("%s: Waiting for target group %s to be online", a.Name(), *targetGroup.id)
				if targetGroup.online(a) == true {
					break
				}
				time.Sleep(5 * time.Second)
			}
		}

		if targetGroup.DesiredTargetGroup == nil {
			lb.Listeners = lb.Listeners.purgeTargetGroupArn(a, targetGroup.CurrentTargetGroup.TargetGroupArn)
			targetGroup.delete(a)
			continue
		}
		tg = append(tg, targetGroup)
	}

	lb.TargetGroups = tg
	return nil
}

func (t TargetGroups) delete(a *albIngress) error {
	errors := false
	for _, targetGroup := range t {
		if err := targetGroup.delete(a); err != nil {
			glog.Infof("%s: Unable to delete target group %s: %s",
				a.Name(),
				*targetGroup.CurrentTargetGroup.TargetGroupArn,
				err)
			errors = true
		}
	}
	if errors {
		return fmt.Errorf("There were errors deleting target groups")
	}
	return nil
}
