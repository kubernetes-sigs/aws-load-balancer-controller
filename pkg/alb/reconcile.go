package alb

type ReconcileOptions struct {
	disableRoute53 bool
	Eventf         func(string, string, string, ...interface{})
	loadbalancer   *LoadBalancer
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetDisableRoute53(b bool) *ReconcileOptions {
	r.disableRoute53 = b
	return r
}

func (r *ReconcileOptions) SetEventf(f func(string, string, string, ...interface{})) *ReconcileOptions {
	r.Eventf = f
	return r
}
