package controller

import (
	"fmt"

	"github.com/golang/glog"
)

type Listeners []*Listener

func (l Listeners) find(listener *Listener) int {
	for p, v := range l {
		if listener.Equals(v.CurrentListener) {
			return p
		}
	}
	return -1
}

// Meant to be called when we delete a targetgroup and just need to lose references to our listeners
func (l Listeners) purgeTargetGroupArn(a *ALBIngress, arn *string) Listeners {
	var listeners Listeners
	for _, listener := range l {
		// TODO: do we ever have more default actions?
		if *listener.CurrentListener.DefaultActions[0].TargetGroupArn != *arn {
			listeners = append(listeners, listener)
		}
	}
	return listeners
}

func (l Listeners) modify(a *ALBIngress, lb *LoadBalancer) error {
	var li Listeners
	for _, targetGroup := range lb.TargetGroups {
		for _, listener := range lb.Listeners {
			if listener.DesiredListener == nil {
				listener.delete(a)
				continue
			}
			if err := listener.modify(a, lb, targetGroup); err != nil {
				return err
			}
			li = append(li, listener)
		}
	}
	lb.Listeners = li
	return nil
}

func (l Listeners) delete(a *ALBIngress) error {
	errors := false
	for _, listener := range l {
		if err := listener.delete(a); err != nil {
			glog.Infof("%s: Unable to delete listener %s: %s",
				a.Name(),
				*listener.CurrentListener.ListenerArn,
				err)
		}
	}
	if errors {
		return fmt.Errorf("There were errors deleting listeners")
	}
	return nil
}
