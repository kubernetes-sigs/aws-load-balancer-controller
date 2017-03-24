package controller

type LoadBalancers []*LoadBalancer

func (l LoadBalancers) find(lb *LoadBalancer) int {
	for i, lbi := range l {
		if *lb.id == *lbi.id {
			return i
		}
	}
	return -1
}
