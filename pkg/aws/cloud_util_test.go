package aws

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUpdateTrackedTargets(t *testing.T) {
	testCases := []struct {
		name                string
		clusterName         string
		expectedSessionName string
	}{
		{
			name:                "no mods",
			clusterName:         "my-cluster-name",
			expectedSessionName: "AWS-LBC-my-cluster-name",
		},
		{
			name:                "mix lower and upper case",
			clusterName:         "My-ClUsTeR-name",
			expectedSessionName: "AWS-LBC-My-ClUsTeR-name",
		},
		{
			name:                "with legal characters",
			clusterName:         "my_cluster-name=foo,something@here.",
			expectedSessionName: "AWS-LBC-my_cluster-name=foo,something@here.",
		},
		{
			name:                "with illegal characters",
			clusterName:         "my&*&*cluster()!(&name",
			expectedSessionName: "AWS-LBC-myclustername",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateAssumeRoleSessionName(tc.clusterName)
			assert.Equal(t, tc.expectedSessionName, result)
		})
	}
}
