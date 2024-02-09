package metrics

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func Test_statusCodeForRequest(t *testing.T) {
	type args struct {
		r *request.Request
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "requests without http response",
			args: args{
				r: &request.Request{},
			},
			want: "0",
		},
		{
			name: "requests with http response",
			args: args{
				r: &request.Request{
					HTTPResponse: &http.Response{
						StatusCode: 200,
					},
				},
			},
			want: "200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusCodeForRequest(tt.args.r)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_errorCodeForRequest(t *testing.T) {
	type args struct {
		r *request.Request
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "requests without error",
			args: args{
				r: &request.Request{},
			},
			want: "",
		},
		{
			name: "requests with internal error",
			args: args{
				r: &request.Request{
					Error: errors.New("oops, some internal error"),
				},
			},
			want: "internal",
		},
		{
			name: "requests with aws error",
			args: args{
				r: &request.Request{
					Error: awserr.New("NotFoundException", "", nil),
				},
			},
			want: "NotFoundException",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errorCodeForRequest(tt.args.r)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_operationForRequest(t *testing.T) {
	type args struct {
		r *request.Request
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "requests without operation",
			args: args{
				r: &request.Request{},
			},
			want: "?",
		},
		{
			name: "requests with operation",
			args: args{
				r: &request.Request{
					Operation: &request.Operation{
						Name: "DescribeMesh",
					},
				},
			},
			want: "DescribeMesh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := operationForRequest(tt.args.r)
			assert.Equal(t, tt.want, got)
		})
	}
}
