package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_StatusEC2(t *testing.T) {
	for _, tc := range []struct {
		Name          string
		Error         error
		ExpectedError error
	}{
		{
			Name:          "No error from API call",
			Error:         nil,
			ExpectedError: nil,
		},
		{
			Name:          "Error from API call",
			Error:         errors.New("Some API error"),
			ExpectedError: errors.New("[ec2.DescribeTagsWithContext]: Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ec2svc := &mocks.EC2API{}
			ec2svc.On("DescribeTagsWithContext", context.TODO(), &ec2.DescribeTagsInput{MaxResults: aws.Int64(1)}).Return(nil, tc.Error)

			cloud := &Cloud{
				ec2: ec2svc,
			}

			err := cloud.StatusEC2()()
			assert.Equal(t, tc.ExpectedError, err)
			ec2svc.AssertExpectations(t)
		})
	}
}
