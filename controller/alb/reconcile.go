package alb

import (
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
)

type ReconcileOptions struct {
	disableRoute53 bool
	ingress        *extensions.Ingress
	recorder       record.EventRecorder
	loadbalancer   *LoadBalancer
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetDisableRoute53(b bool) *ReconcileOptions {
	r.disableRoute53 = b
	return r
}

func (r *ReconcileOptions) SetIngress(i *extensions.Ingress) *ReconcileOptions {
	r.ingress = i
	return r
}

func (r *ReconcileOptions) SetRecorder(recorder record.EventRecorder) *ReconcileOptions {
	r.recorder = recorder
	return r
}

func (r *ReconcileOptions) Eventf(eventtype, reason, messageFmt string, args ...interface{}) {
	if r.ingress == nil {
		return
	}
	r.recorder.Eventf(r.ingress, eventtype, reason, messageFmt, args...)
}
