package equality

import (
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/sets"
	"reflect"
)

// IgnoreOtherFields is option that only compare specific structures fields.
func IgnoreOtherFields(typ interface{}, fields ...string) cmp.Option {
	t := reflect.TypeOf(typ)
	fieldsSet := sets.NewString(fields...)
	return cmp.FilterPath(func(path cmp.Path) bool {
		if len(path) < 2 || path.Index(-2).Type() != t {
			return false
		}
		ps, ok := path.Last().(cmp.StructField)
		if !ok || fieldsSet.Has(ps.Name()) {
			return false
		}
		return true
	}, cmp.Ignore())
}
