package throttle

import (
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
)

func Test_matchService(t *testing.T) {
	type args struct {
		serviceID string
	}
	tests := []struct {
		name string
		args args
		req  *request.Request
		want bool
	}{
		{
			name: "service matches",
			args: args{
				serviceID: "App Mesh",
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
			},
			want: true,
		},
		{
			name: "service mismatches",
			args: args{
				serviceID: "App Mesh",
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "S3",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predict := matchService(tt.args.serviceID)
			got := predict(tt.req)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_matchServiceOperation(t *testing.T) {
	type args struct {
		serviceID string
		operation string
	}
	tests := []struct {
		name string
		args args
		req  *request.Request
		want bool
	}{
		{
			name: "operation matches",
			args: args{
				serviceID: "App Mesh",
				operation: "CreateMesh",
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
				Operation: &request.Operation{
					Name: "CreateMesh",
				},
			},
			want: true,
		},
		{
			name: "operation mismatches",
			args: args{
				serviceID: "App Mesh",
				operation: "CreateMesh",
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
				Operation: &request.Operation{
					Name: "DescribeMesh",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predict := matchServiceOperation(tt.args.serviceID, tt.args.operation)
			got := predict(tt.req)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_matchServiceOperationPattern(t *testing.T) {
	type args struct {
		serviceID    string
		operationPtn *regexp.Regexp
	}
	tests := []struct {
		name string
		args args
		req  *request.Request
		want bool
	}{
		{
			name: "operationPtn matches - case 1",
			args: args{
				serviceID:    "App Mesh",
				operationPtn: regexp.MustCompile("Create"),
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
				Operation: &request.Operation{
					Name: "CreateMesh",
				},
			},
			want: true,
		},
		{
			name: "operationPtn matches - case 2",
			args: args{
				serviceID:    "App Mesh",
				operationPtn: regexp.MustCompile("Create.*"),
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
				Operation: &request.Operation{
					Name: "CreateMesh",
				},
			},
			want: true,
		},
		{
			name: "operationPtn matches - case 3",
			args: args{
				serviceID:    "App Mesh",
				operationPtn: regexp.MustCompile("^Create"),
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
				Operation: &request.Operation{
					Name: "CreateMesh",
				},
			},
			want: true,
		},
		{
			name: "operationPtn matches - case 4",
			args: args{
				serviceID:    "App Mesh",
				operationPtn: regexp.MustCompile("Mesh"),
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
				Operation: &request.Operation{
					Name: "CreateMesh",
				},
			},
			want: true,
		},
		{
			name: "operationPtn mismatches",
			args: args{
				serviceID:    "App Mesh",
				operationPtn: regexp.MustCompile("Describe"),
			},
			req: &request.Request{
				ClientInfo: metadata.ClientInfo{
					ServiceID: "App Mesh",
				},
				Operation: &request.Operation{
					Name: "CreateMesh",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predict := matchServiceOperationPattern(tt.args.serviceID, tt.args.operationPtn)
			got := predict(tt.req)
			assert.Equal(t, tt.want, got)
		})
	}
}
