package algorithm

// GetMin returns the smaller of x or y
func GetMin(x, y int) int {
	if x < y {
		return x
	}
	return y
}
