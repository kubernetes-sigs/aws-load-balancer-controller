package sg

// Information about securityGroup on LoadBalancer
type LbAttachmentInfo struct {
	// The managed securityGroupID. It will be empty when securityGroups are external-managed via annotation `alb.ingress.kubernetes.io/security-groups`
	ManagedGroupID string
}

// NameGenerator provides name generation functionality for sg package.
type NameGenerator interface {
	// NameLBSG generates name for managed securityGroup that will be attached to LoadBalancer.
	NameLBSG(namespace string, ingressName string) string

	// NameLBSG generates name for managed securityGroup that will be attached to EC2 instances.
	NameInstanceSG(namespace string, ingressName string) string
}

// TagGenerator provides tag generation functionality for sg package.
type TagGenerator interface {
	// TagLBSG generates tags for managed securityGroup that will be attached to LoadBalancer.
	TagLBSG(namespace string, ingressName string) map[string]string

	// TagInstanceSG generates tags for managed securityGroup that will be attached to EC2 instances.
	TagInstanceSG(namespace string, ingressName string) map[string]string
}

// NameTagGenerator is combination of NameGenerator and TagGenerator
type NameTagGenerator interface {
	NameGenerator
	TagGenerator
}
