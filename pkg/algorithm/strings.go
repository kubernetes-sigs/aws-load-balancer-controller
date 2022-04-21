package algorithm

import "k8s.io/apimachinery/pkg/util/sets"

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

func DiffStringSlice(first, second []string) ([]*string, []*string, []*string) {
	firstSet := sets.NewString(first...)
	secondSet := sets.NewString(second...)

	matchFirst := make([]*string, 0)
	matchBoth := make([]*string, 0)
	matchSecond := make([]*string, 0)
	for _, elem := range firstSet.Difference(secondSet).List() {
		elem := elem
		matchFirst = append(matchFirst, &elem)
	}
	for _, elem := range secondSet.Difference(firstSet).List() {
		elem := elem
		matchSecond = append(matchSecond, &elem)
	}
	for _, elem := range firstSet.Intersection(secondSet).List() {
		elem := elem
		matchBoth = append(matchBoth, &elem)
	}
	return matchFirst, matchBoth, matchSecond
}
