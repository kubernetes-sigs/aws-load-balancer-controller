package utils

// MapFindFirst get from list of maps until first found.
func MapFindFirst(key string, maps ...map[string]string) (string, bool) {
	for _, m := range maps {
		v, ok := m[key]
		if ok {
			return v, ok
		}
	}
	return "", false
}
