package tracking

// TagFilter presents tag filter for multiple TagKeys.
// the TagKey is represented by mapKey, and TagValues is represented by tagValues
// if the TagValue is empty, then it only requires the TagKey presents.
// if the TagValue is not empty, then it requires TagKey presents and one of the TagValue matches.
type TagFilter map[string][]string

func (f TagFilter) Matches(tags map[string]string) bool {
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

// TagsAsTagFilter constructs TagFilter from Tags.
func TagsAsTagFilter(tags map[string]string) TagFilter {
	tagFilter := make(TagFilter, len(tags))
	for key, value := range tags {
		tagFilter[key] = []string{value}
	}
	return tagFilter
}
