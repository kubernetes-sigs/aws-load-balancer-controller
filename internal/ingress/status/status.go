/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status

import (
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	pool "gopkg.in/go-playground/pool.v3"
	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/task"
)

const (
	updateInterval = 60 * time.Second
)

// Sync ...
type Sync interface {
	Run()
	Shutdown()
}

type ingressLister interface {
	// ListIngresses returns the list of Ingresses
	ListIngresses() []*extensions.Ingress
}

// Config ...
type Config struct {
	Client clientset.Interface

	ElectionID string

	IngressLister ingressLister

	DefaultIngressClass string
	IngressClass        string

	RunningConfig *ingress.Configuration
}

// statusSync keeps the status IP in each Ingress rule updated executing a periodic check
// in all the defined rules. To simplify the process leader election is used so the update
// is executed only in one node (Ingress controllers can be scaled to more than one)
// If the controller is running with the flag --publish-service (with a valid service)
// the IP address behind the service is used, if it is running with the flag
// --publish-status-address, the address specified in the flag is used, if neither of the
// two flags are set, the source is the IP/s of the node/s
type statusSync struct {
	Config
	// pod contains runtime information about this pod
	pod *k8s.PodInfo

	elector *leaderelection.LeaderElector
	// workqueue used to keep in sync the status IP/s
	// in the Ingress rules
	syncQueue *task.Queue
}

// Run starts the loop to keep the status in sync
func (s statusSync) Run() {
	s.elector.Run()
}

// Shutdown stop the sync. In case the instance is the leader it will remove the current IP
// if there is no other instances running.
func (s statusSync) Shutdown() {
	go s.syncQueue.Shutdown()
}

func (s *statusSync) sync(key interface{}) error {
	if s.syncQueue.IsShuttingDown() {
		glog.V(2).Infof("skipping Ingress status update (shutting down in progress)")
		return nil
	}

	s.updateStatus()

	return nil
}

func (s statusSync) keyfunc(input interface{}) (interface{}, error) {
	return input, nil
}

// NewStatusSyncer returns a new Sync instance
func NewStatusSyncer(config Config) Sync {
	pod, err := k8s.GetPodDetails(config.Client)
	if err != nil {
		glog.Fatalf("unexpected error obtaining pod information: %v", err)
	}

	st := statusSync{
		pod: pod,

		Config: config,
	}
	st.syncQueue = task.NewCustomTaskQueue(st.sync, st.keyfunc)

	// we need to use the defined ingress class to allow multiple leaders
	// in order to update information about ingress status
	electionID := fmt.Sprintf("%v-%v", config.ElectionID, config.DefaultIngressClass)
	if config.IngressClass != "" {
		electionID = fmt.Sprintf("%v-%v", config.ElectionID, config.IngressClass)
	}

	callbacks := leaderelection.LeaderCallbacks{
		OnStartedLeading: func(stop <-chan struct{}) {
			glog.V(2).Infof("I am the new status update leader")
			go st.syncQueue.Run(time.Second, stop)
			// when this instance is the leader we need to enqueue
			// an item to trigger the update of the Ingress status.
			wait.PollUntil(updateInterval, func() (bool, error) {
				st.syncQueue.EnqueueTask(task.GetDummyObject("sync status"))
				return false, nil
			}, stop)
		},
		OnStoppedLeading: func() {
			glog.V(2).Infof("I am not status update leader anymore")
		},
		OnNewLeader: func(identity string) {
			glog.Infof("new leader elected: %v", identity)
		},
	}

	broadcaster := record.NewBroadcaster()
	hostname, _ := os.Hostname()

	recorder := broadcaster.NewRecorder(scheme.Scheme, apiv1.EventSource{
		Component: "ingress-leader-elector",
		Host:      hostname,
	})

	lock := resourcelock.ConfigMapLock{
		ConfigMapMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: electionID},
		Client:        config.Client.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      pod.Name,
			EventRecorder: recorder,
		},
	}

	ttl := 30 * time.Second
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          &lock,
		LeaseDuration: ttl,
		RenewDeadline: ttl / 2,
		RetryPeriod:   ttl / 4,
		Callbacks:     callbacks,
	})

	if err != nil {
		glog.Fatalf("unexpected error starting leader election: %v", err)
	}

	st.elector = le
	return st
}

// updateStatus changes the status information of Ingress rules
func (s *statusSync) updateStatus() {
	ings := s.IngressLister.ListIngresses()

	p := pool.NewLimited(10)
	defer p.Close()

	batch := p.Batch()

	for _, ing := range ings {
		batch.Queue(runUpdate(ing, s.Client, s.RunningConfig))
	}

	batch.QueueComplete()
	batch.WaitAll()
}

func runUpdate(ing *extensions.Ingress, client clientset.Interface, rc *ingress.Configuration) pool.WorkFunc {
	return func(wu pool.WorkUnit) (interface{}, error) {
		if wu.IsCancelled() {
			return nil, nil
		}

		var current []apiv1.LoadBalancerIngress
		if _, i := rc.Ingresses.FindByID(k8s.MetaNamespaceKey(ing)); i != nil {
			hostnames, err := i.Hostnames()
			if err == nil {
				current = hostnames
			}
		}

		if ingressSliceEqual(ing.Status.LoadBalancer.Ingress, current) {
			glog.V(3).Infof("skipping update of Ingress %v/%v (no change)", ing.Namespace, ing.Name)
			return true, nil
		}

		ingClient := client.ExtensionsV1beta1().Ingresses(ing.Namespace)

		currIng, err := ingClient.Get(ing.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("unexpected error searching Ingress %v/%v", ing.Namespace, ing.Name))
		}

		glog.Infof("updating Ingress %v/%v status to %v", currIng.Namespace, currIng.Name, current)
		currIng.Status.LoadBalancer.Ingress = current
		_, err = ingClient.UpdateStatus(currIng)
		if err != nil {
			glog.Warningf("error updating ingress rule: %v", err)
		}

		return true, nil
	}
}

func ingressSliceEqual(lhs, rhs []apiv1.LoadBalancerIngress) bool {
	if len(lhs) != len(rhs) {
		return false
	}

	for i := range lhs {
		if lhs[i].Hostname != rhs[i].Hostname {
			return false
		}
		if lhs[i].IP != rhs[i].IP {
			return false
		}
	}
	return true
}
