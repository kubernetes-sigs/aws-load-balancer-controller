package lb

// LoadBalancer contains information of LoadBalancer in AWS
type LoadBalancer struct {
	Arn     string
	DNSName string
}

// NameGenerator generates name for loadBalancer resources
type NameGenerator interface {
	NameLB(namespace string, serviceName string) string
}

// TagGenerator generates tags for loadBalancer resources
type TagGenerator interface {
	TagLB(namespace string, serviceName string) map[string]string
}

// NameTagGenerator combines NameGenerator & TagGenerator
type NameTagGenerator interface {
	NameGenerator
	TagGenerator
}
