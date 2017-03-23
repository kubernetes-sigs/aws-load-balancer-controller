package controller

type Rules []*Rule

func (r Rules) find(rule *Rule) int {
	for p, v := range r {
		if rule.Equals(v.CurrentRule) {
			return p
		}
	}
	return -1
}
