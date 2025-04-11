package loadbalancer

import elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"

type Mutator interface {
	Mutate(spec *elbv2model.LoadBalancerSpec) error
}
