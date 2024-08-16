package algorithm

import "cmp"

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
