package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_StatusIAM(t *testing.T) {
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
			ExpectedError: errors.New("[iam.ListServerCertificatesWithContext]: Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			iamsvc := &mocks.IAMAPI{}
			iamsvc.On("ListServerCertificatesWithContext", context.TODO(), &iam.ListServerCertificatesInput{MaxItems: aws.Int64(1)}).Return(nil, tc.Error)

			cloud := &Cloud{
				iam: iamsvc,
			}

			err := cloud.StatusIAM()()
			assert.Equal(t, tc.ExpectedError, err)
			iamsvc.AssertExpectations(t)
		})
	}
}
