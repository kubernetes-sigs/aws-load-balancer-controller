package alb

type ReconcileOptions struct {
	Eventf       func(string, string, string, ...interface{})
	loadbalancer *LoadBalancer
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetLoadBalancer(loadbalancer *LoadBalancer) *ReconcileOptions {
	r.loadbalancer = loadbalancer
	return r
}

func (r *ReconcileOptions) SetEventf(f func(string, string, string, ...interface{})) *ReconcileOptions {
	r.Eventf = f
	return r
}
