package aws

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws/request"
)

func newReq(data interface{}, err error) *request.Request {
	return &request.Request{
		Data:        data,
		HTTPRequest: &http.Request{},
		Operation:   &request.Operation{Paginator: &request.Paginator{}},
		Error:       err,
	}
}
