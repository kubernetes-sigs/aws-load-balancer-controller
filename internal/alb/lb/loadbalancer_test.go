package lb

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_ResolveSecurityGroupNames(t *testing.T) {
	idmap := map[string]string{
		"sg1": "sg-123456",
		"sg2": "sg-456789",
	}
	for _, tc := range []struct {
		Name   string
		Input  []string
		Output []string

		GetSecurityGroupsByNameInput  []string
		GetSecurityGroupsByNameOutput []*ec2.SecurityGroup
		GetSecurityGroupsByNameError  error

		ExpectedError error
	}{
		{
			Name: "empty input, empty output",
		},
		{
			Name:   "single resolved 'sg-' input",
			Input:  []string{idmap["sg1"]},
			Output: []string{idmap["sg1"]},
		},
		{
			Name:                          "single named 'sg1' input",
			Input:                         []string{"sg1"},
			Output:                        []string{idmap["sg1"]},
			GetSecurityGroupsByNameInput:  []string{"sg1"},
			GetSecurityGroupsByNameOutput: []*ec2.SecurityGroup{{GroupId: aws.String(idmap["sg1"])}},
		},
		{
			Name:                          "mixed named and unnamed input",
			Input:                         []string{"sg1", idmap["sg2"]},
			Output:                        []string{idmap["sg1"], idmap["sg2"]},
			GetSecurityGroupsByNameInput:  []string{"sg1"},
			GetSecurityGroupsByNameOutput: []*ec2.SecurityGroup{{GroupId: aws.String(idmap["sg1"])}},
		},
		{
			Name:                          "a sg name that doesn't resolve",
			Input:                         []string{"sg1", idmap["sg2"]},
			Output:                        []string{idmap["sg2"]},
			GetSecurityGroupsByNameInput:  []string{"sg1"},
			GetSecurityGroupsByNameOutput: []*ec2.SecurityGroup{},
			ExpectedError:                 errors.New("not all security groups were resolvable, (sg1,sg-456789 != sg-456789)"),
		},
		{
			Name:                         "Error from GetSecurityGroupsByName",
			Input:                        []string{"sg1", idmap["sg2"]},
			Output:                       []string{idmap["sg2"]},
			GetSecurityGroupsByNameInput: []string{"sg1"},
			GetSecurityGroupsByNameError: errors.New("Some API error"),
			ExpectedError:                errors.New("Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.GetSecurityGroupsByNameInput != nil {
				cloud.On("GetSecurityGroupsByName",
					context.TODO(),
					tc.GetSecurityGroupsByNameInput).Return(
					tc.GetSecurityGroupsByNameOutput,
					tc.GetSecurityGroupsByNameError,
				)
			}

			controller := &defaultController{
				cloud: cloud,
			}

			out, err := controller.resolveSecurityGroupNames(ctx, tc.Input)
			sort.Strings(tc.Output)
			sort.Strings(out)
			assert.Equal(t, tc.Output, out)
			assert.Equal(t, tc.ExpectedError, err)
			cloud.AssertExpectations(t)
		})
	}
}
