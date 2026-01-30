package algorithm

// ChunkStrings will split slice of String into chunks
func ChunkStrings(targets []string, chunkSize int) [][]string {
	var chunks [][]string
	for i := 0; i < len(targets); i += chunkSize {
		end := i + chunkSize
		if end > len(targets) {
			end = len(targets)
		}
		chunks = append(chunks, targets[i:end])
	}
	return chunks
}

// LevenshteinDistance calculates the minimum number of single-character edits
// (insertions, deletions, substitutions) required to change one string into another.
func LevenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use two rows instead of full matrix for space efficiency
	prevRow := make([]int, len(b)+1)
	currRow := make([]int, len(b)+1)

	for j := range prevRow {
		prevRow[j] = j
	}

	for i := 1; i <= len(a); i++ {
		currRow[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			currRow[j] = min(
				currRow[j-1]+1,    // insertion
				prevRow[j]+1,      // deletion
				prevRow[j-1]+cost, // substitution
			)
		}
		prevRow, currRow = currRow, prevRow
	}

	return prevRow[len(b)]
}

// StringsSimilar returns true if two strings have a similarity ratio at or above
// the given threshold. Similarity is calculated as 1 - (edit_distance / max_length).
// This is useful for comparing strings that may have dynamic parts like IDs or timestamps.
func StringsSimilar(a, b string, threshold float64) bool {
	if a == b {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	distance := LevenshteinDistance(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)
	return similarity >= threshold
}
