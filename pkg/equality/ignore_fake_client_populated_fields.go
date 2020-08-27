package equality

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IgnoreFakeClientPopulatedFields is an option to ignore fields populated by fakeK8sClient for a comparison.
// Use this when comparing k8s objects in test cases.
// These fields are ignored: TypeMeta and ObjectMeta.ResourceVersion
func IgnoreFakeClientPopulatedFields() cmp.Option {
	return cmp.Options{
		// ignore unset fields in left hand side
		cmpopts.IgnoreTypes(metav1.TypeMeta{}),
		cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion"),
	}
}
