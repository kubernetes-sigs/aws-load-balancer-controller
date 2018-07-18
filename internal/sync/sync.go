package sync

import (
	"sync"

	"github.com/golang/glog"
)

type RWMutex struct {
	m sync.RWMutex
}

func (r *RWMutex) RLock() {
	if glog.V(3) {
		glog.InfoDepth(3, "Requesting RLock.")
	}
	r.m.RLock()
	if glog.V(3) {
		glog.InfoDepth(3, "RLock was available.")
	}
}

func (r *RWMutex) Lock() {
	if glog.V(3) {
		glog.InfoDepth(3, "Requesting Lock.")
	}
	r.m.Lock()
	if glog.V(3) {
		glog.InfoDepth(3, "Lock was available.")
	}
}

func (r *RWMutex) RUnlock() {
	r.m.RUnlock()
	if glog.V(3) {
		glog.InfoDepth(3, "RUnlock'd.")
	}
}

func (r *RWMutex) Unlock() {
	r.m.Unlock()
	if glog.V(3) {
		glog.InfoDepth(3, "Unlock'd.")
	}
}
