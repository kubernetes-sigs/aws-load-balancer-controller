package equality

import (
	"github.com/google/go-cmp/cmp"
	"reflect"
)

// FilterField return a new option that only apply specified option to specific struct field.
func FilterField(typ interface{}, field string, opt cmp.Option) cmp.Option {
	t := reflect.TypeOf(typ)
	return cmp.FilterPath(func(path cmp.Path) bool {
		if len(path) < 2 || path.Index(-2).Type() != t {
			return false
		}
		ps, ok := path.Last().(cmp.StructField)
		if !ok || field != ps.Name() {
			return false
		}
		return true
	}, opt)
}
