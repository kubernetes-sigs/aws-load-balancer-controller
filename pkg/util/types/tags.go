package types

import (
	"github.com/aws/aws-sdk-go/service/ec2"
)

type EC2Tags []*ec2.Tag

func (t EC2Tags) Get(s string) (string, bool) {
	for _, tag := range t {
		if *tag.Key == s {
			return *tag.Value, true
		}
	}
	return "", false
}

// desiredTags presents tag filter for multiple TagKeys.
// the TagKey is represented by mapKey, and TagValues is represented by tagValues
// if the TagValue is empty, then it only requires the TagKey presents.
// if the TagValue is not empty, then it requires TagKey presents and one of the TagValue matches.
func Matches(checkedTags map[string]string, desiredTags map[string][]string) bool {
	for key, desiredValues := range desiredTags {
		actualValue, ok := checkedTags[key]
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
