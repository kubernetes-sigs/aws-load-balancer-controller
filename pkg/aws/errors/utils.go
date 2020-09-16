package errors

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/pkg/errors"
)

func IsELBV2TargetGroupNotFoundError(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == "TargetGroupNotFound"
	}
	return false
}

func IsEC2SecurityGroupNotFoundError(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == "InvalidGroup.NotFound"
	}
	return false
}
