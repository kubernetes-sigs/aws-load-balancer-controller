package ingress

const (
	ingressClassALB = "alb"
)

// ClassAnnotationMatcher tests whether the kubernetes.io/ingress.class annotation on Ingresses matches the IngressClass of this controller.
type ClassAnnotationMatcher interface {
	Matches(ingClassAnnotation string) bool
}

// NewDefaultClassAnnotationMatcher constructs new defaultClassAnnotationMatcher.
func NewDefaultClassAnnotationMatcher(ingressClass string) *defaultClassAnnotationMatcher {
	return &defaultClassAnnotationMatcher{
		ingressClass: ingressClass,
	}
}

var _ ClassAnnotationMatcher = &defaultClassAnnotationMatcher{}

// default implementation for ClassAnnotationMatcher, which supports users to provide a single custom IngressClass.
type defaultClassAnnotationMatcher struct {
	ingressClass string
}

func (m *defaultClassAnnotationMatcher) Matches(ingClassAnnotation string) bool {
	if m.ingressClass == "" && ingClassAnnotation == ingressClassALB {
		return true
	}
	return ingClassAnnotation == m.ingressClass
}
