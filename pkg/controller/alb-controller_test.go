package controller

import (
	"regexp"
	"testing"
)

const albRegex = "^[a-zA-Z0-9]+$"

func TestALBNamePrefixGeneratedCompliesWithALB(t *testing.T) {
	expectedName := "clustername" // dashes removed and limited to 11 chars
	in := "cluster-name-hello"
	actualName, err := cleanClusterName(in)
	if err != nil {
		t.Errorf("Error returned atttempted to create ALB prefix. Error: %s", err.Error())
	}

	if actualName != expectedName {
		t.Errorf("ALBNamePrefix generated incorrectly was: %s | expected: %s",
			actualName, expectedName)
	}

	// sanity check on expectedName; ensures it's compliant with ALB naming
	match, err := regexp.MatchString(albRegex, expectedName)
	if err != nil {
		t.Errorf("Failed to parse regex for test. Likley an issues with the test. Regex: %s",
			albRegex)
	}
	if !match {
		t.Errorf("Expected name was not compliant with AWS-ALB naming restrictions. Could be "+
			"issue with test. expectedName: %s, compliantRegexTest: %s", expectedName, albRegex)
	}
}
