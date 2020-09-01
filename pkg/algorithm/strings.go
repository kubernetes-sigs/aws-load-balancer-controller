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
