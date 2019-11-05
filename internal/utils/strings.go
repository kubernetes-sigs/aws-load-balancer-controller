package utils

import (
	"fmt"
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
			return nil, fmt.Errorf("invalid mapStringBool: %v", s)
		}
		k := strings.TrimSpace(parts[0])
		v, err := strconv.ParseBool(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid mapStringBool: %v", s)
		}
		result[k] = v
	}
	return result, nil
}

func SplitStringSlice(strSlice []string, chunkSize int) [][]string {
	sliceLen := len(strSlice)
	var result [][]string
	for i := 0; i < sliceLen; i += chunkSize {
		end := i + chunkSize
		if end > sliceLen {
			end = sliceLen
		}
		result = append(result, strSlice[i:end])
	}
	return result
}
