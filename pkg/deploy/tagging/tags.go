package tagging

// TagFilter presents filter to tags.
type TagFilter interface {
	// Matches test whether TagFilter matches Tags.
	Matches(map[string]string) bool
}

var _ TagFilter = MultiValueTagFilter{}

// MultiValueTagFilter presents tag filter for multiple TagKeys.
// the TagKey is represented by mapKey, and TagValues is represented by tagValues
// if the TagValue is empty, then it only requires the TagKey presents.
// if the TagValue is not empty, then it requires TagKey presents and one of the TagValue matches.
type MultiValueTagFilter map[string][]string

func (f MultiValueTagFilter) Matches(tags map[string]string) bool {
	for key, desiredValues := range f {
		actualValue, ok := tags[key]
		if !ok {
			return false
		}
		if len(desiredValues) == 0 {
			continue
		}
		matchedAnyValue := false
		for _, desiredValue := range desiredValues {
			if desiredValue == actualValue {
				matchedAnyValue = true
				break
			}
		}
		if !matchedAnyValue {
			return false
		}
	}
	return true
}

// TagsAsMultiValueTagFilter constructs MultiValueTagFilter from Tags.
func TagsAsMultiValueTagFilter(tags map[string]string) MultiValueTagFilter {
	tagFilter := make(MultiValueTagFilter, len(tags))
	for key, value := range tags {
		tagFilter[key] = []string{value}
	}
	return tagFilter
}
