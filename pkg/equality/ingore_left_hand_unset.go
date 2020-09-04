package equality

import (
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/sets"
	"reflect"
)

// IgnoreLeftHandUnset is an option that ignores struct fields that are unset on the left hand side of a comparison.
// Note:
//	 1. for map and slices, only nil value is considered to be unset, non-nil but empty is not considered as unset.
//   2. for struct pointers, nil value is considered to be unset
func IgnoreLeftHandUnset(typ interface{}, fields ...string) cmp.Option {
	t := reflect.TypeOf(typ)
	fieldsSet := sets.NewString(fields...)
	return cmp.FilterPath(func(path cmp.Path) bool {
		if len(path) < 2 || path.Index(-2).Type() != t {
			return false
		}
		ps, ok := path.Last().(cmp.StructField)
		if !ok || !fieldsSet.Has(ps.Name()) {
			return false
		}

		v1, _ := ps.Values()
		switch v1.Kind() {
		case reflect.Slice, reflect.Map, reflect.Ptr:
			return v1.IsNil()
		}
		return false
	}, cmp.Ignore())
}
