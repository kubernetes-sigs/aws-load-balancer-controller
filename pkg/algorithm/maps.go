package algorithm

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

// MergeStringMap will merge multiple map[string]string into single one.
// The merge is executed for maps argument in sequential order, if a key already exists, the value from previous map is kept.
// e.g. MergeStringMap(map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "3", "d": "4"}) == map[string]string{"a": "1", "b": "2", "d": "4"}
func MergeStringMap(maps ...map[string]string) map[string]string {
	ret := make(map[string]string)
	for _, _map := range maps {
		for k, v := range _map {
			if _, ok := ret[k]; !ok {
				ret[k] = v
			}
		}
	}
	return ret
}

// DiffStringMap will diff desired with current.
// Returns the k/v that need to be add/updated and k/v that need to be deleted to make current match desired.
func DiffStringMap(desired map[string]string, current map[string]string) (map[string]string, map[string]string) {
	modify := make(map[string]string)
	remove := make(map[string]string)

	for key, desiredVal := range desired {
		currentVal, ok := current[key]
		if !ok || currentVal != desiredVal {
			modify[key] = desiredVal
		}
	}

	for key, currentVal := range current {
		if _, ok := desired[key]; !ok {
			remove[key] = currentVal
		}
	}

	return modify, remove
}
