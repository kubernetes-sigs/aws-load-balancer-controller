package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCloud_StatusACM(t *testing.T) {
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
			ExpectedError: errors.New("[acm.ListCertificates]: Some API error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			acmsvc := &mocks.ACMAPI{}
			acmsvc.On("ListCertificatesWithContext", context.TODO(), &acm.ListCertificatesInput{MaxItems: aws.Int64(1)}).Return(nil, tc.Error)

			cloud := &Cloud{
				acm: acmsvc,
			}

			err := cloud.StatusACM()()
			assert.Equal(t, tc.ExpectedError, err)
			acmsvc.AssertExpectations(t)
		})
	}
}
