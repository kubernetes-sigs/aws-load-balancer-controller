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

// isDiffStringSlice reports whether these two slices contain the same strings
// ignoring their order
// string equality is based on Strings.Equal
// TODO: this could probably be more efficient
func IsDiffStringSlice(one []string, two []string) bool {
	if len(one) != len(two) {
		return false
	}

	for _, item := range one {
		if !slices.Contains(two, item) {
			return false
		}
	}

	for _, item := range two {
		if !slices.Contains(one, item) {
			return false
		}
	}

	return true
}
