package sync

import (
	"sync"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

var logger *log.Logger

func init() {
	logger = log.New("sync")
}

type RWMutex struct {
	m sync.RWMutex
}

func (r *RWMutex) RLock() {
	logger.DebugLevelf(3, "Requesting RLock.")
	r.m.RLock()
	logger.DebugLevelf(3, "RLock was available.")
}

func (r *RWMutex) Lock() {
	logger.DebugLevelf(3, "Requesting Lock.")
	r.m.Lock()
	logger.DebugLevelf(3, "Lock was available.")
}

func (r *RWMutex) RUnlock() {
	r.m.RUnlock()
	logger.DebugLevelf(3, "RUnlock")
}

func (r *RWMutex) Unlock() {
	r.m.Unlock()
	logger.DebugLevelf(3, "Unlock")
}
