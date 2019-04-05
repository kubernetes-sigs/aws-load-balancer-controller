package algorithm

import (
	"github.com/pkg/errors"
	"strconv"
	"strings"
)

// SplitMapStringBool parse comma-separated string of key1=value1,key2=value2. value is either true or false
func SplitMapStringBool(str string) (map[string]bool, error) {
	result := make(map[string]bool)
	for _, s := range strings.Split(str, ",") {
		if len(s) == 0 {
			continue
		}
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid mapStringBool: %v", s)
		}
		k := strings.TrimSpace(parts[0])
		v, err := strconv.ParseBool(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, errors.Errorf("invalid mapStringBool: %v", s)
		}
		result[k] = v
	}
	return result, nil
}

// ContainsString returns whether string slice contains specified string.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString returns a new string slice by remove specified string.
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
