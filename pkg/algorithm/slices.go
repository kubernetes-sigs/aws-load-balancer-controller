package algorithm

import (
	"cmp"
	"slices"
)

// RemoveSliceDuplicates returns a copy of the slice without duplicate entries.
func RemoveSliceDuplicates[S ~[]E, E cmp.Ordered](s S) []E {
	result := make([]E, 0, len(s))
	found := make(map[E]struct{}, len(s))

	for _, x := range s {
		if _, ok := found[x]; !ok {
			found[x] = struct{}{}
			result = append(result, x)
		}
	}

	return result
}

// isDiffStringSlice returns true if two slices contain the same elements
// according to strings.Equal, order is ignored
func IsDiffStringSlice(one []string, two []string) bool {
	if len(one) != len(two) {
		return false
	}

	for i := 0; i < len(one); i++ {
		if !slices.Contains(two, one[i]) {
			return false
		}
	}

	return true
}
