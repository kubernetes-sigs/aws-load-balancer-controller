package equality

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// IgnoreStringSliceOrder is option that compare string slices without order.
func IgnoreStringSliceOrder() cmp.Option {
	return cmpopts.SortSlices(func(lhs string, rhs string) bool {
		return lhs < rhs
	})
}
