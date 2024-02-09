package throttle

import (
	"github.com/aws/aws-sdk-go/service/appmesh"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
)

func TestServiceOperationsThrottleConfig_String(t *testing.T) {
	type fields struct {
		value map[string][]throttleConfig
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "non-empty value",
			fields: fields{
				value: map[string][]throttleConfig{
					appmesh.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Describe"),
							r:            4.2,
							burst:        5,
						},
						{
							operationPtn: regexp.MustCompile("CreateMesh"),
							r:            4.2,
							burst:        5,
						},
					},
					servicediscovery.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Describe"),
							r:            4.2,
							burst:        5,
						},
					},
				},
			},
			want: "App Mesh:^Describe=4.2:5,App Mesh:CreateMesh=4.2:5,ServiceDiscovery:^Describe=4.2:5",
		},
		{
			name: "nil value",
			fields: fields{
				value: nil,
			},
			want: "",
		},
		{
			name: "empty value",
			fields: fields{
				value: nil,
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ServiceOperationsThrottleConfig{
				value: tt.fields.value,
			}
			got := c.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServiceOperationsThrottleConfig_Set(t *testing.T) {
	type fields struct {
		value map[string][]throttleConfig
	}
	type args struct {
		val string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    ServiceOperationsThrottleConfig
		wantErr error
	}{
		{
			name: "when default value is nil",
			fields: fields{
				value: nil,
			},
			args: args{
				val: "App Mesh:^Describe=4.2:5,App Mesh:CreateMesh=4.2:6,ServiceDiscovery:^Describe=4.2:7",
			},
			want: ServiceOperationsThrottleConfig{
				value: map[string][]throttleConfig{
					appmesh.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Describe"),
							r:            4.2,
							burst:        5,
						},
						{
							operationPtn: regexp.MustCompile("CreateMesh"),
							r:            4.2,
							burst:        6,
						},
					},
					servicediscovery.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Describe"),
							r:            4.2,
							burst:        7,
						},
					},
				},
			},
		},
		{
			name: "when default value contains non-empty defaults",
			fields: fields{
				value: map[string][]throttleConfig{
					elbv2.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Create"),
							r:            4.2,
							burst:        4,
						},
					},
				},
			},
			args: args{
				val: "App Mesh:^Describe=4.2:5,App Mesh:CreateMesh=4.2:6,ServiceDiscovery:^Describe=4.2:7",
			},
			want: ServiceOperationsThrottleConfig{
				value: map[string][]throttleConfig{
					elbv2.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Create"),
							r:            4.2,
							burst:        4,
						},
					},
					appmesh.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Describe"),
							r:            4.2,
							burst:        5,
						},
						{
							operationPtn: regexp.MustCompile("CreateMesh"),
							r:            4.2,
							burst:        6,
						},
					},
					servicediscovery.ServiceID: {
						{
							operationPtn: regexp.MustCompile("^Describe"),
							r:            4.2,
							burst:        7,
						},
					},
				},
			},
		},
		{
			name: "when val is empty",
			fields: fields{
				value: map[string][]throttleConfig{},
			},
			args: args{
				val: "",
			},
			wantErr: errors.Errorf(" must be formatted as serviceID:operationRegex=rate:burst"),
		},
		{
			name: "when val is not valid format - case 1",
			fields: fields{
				value: map[string][]throttleConfig{},
			},
			args: args{
				val: "a=b=c",
			},
			wantErr: errors.Errorf("a=b=c must be formatted as serviceID:operationRegex=rate:burst"),
		},
		{
			name: "when val is not valid format - case 2",
			fields: fields{
				value: map[string][]throttleConfig{},
			},
			args: args{
				val: "a:b:c=4.2:5",
			},
			wantErr: errors.Errorf("a:b:c must be formatted as serviceID:operationRegex"),
		},
		{
			name: "when val is not valid format - case 3",
			fields: fields{
				value: map[string][]throttleConfig{},
			},
			args: args{
				val: "a:b=4.2:5:6",
			},
			wantErr: errors.Errorf("4.2:5:6 must be formatted as rate:burst"),
		},
		{
			name: "when operationPtn is not valid regex",
			fields: fields{
				value: map[string][]throttleConfig{},
			},
			args: args{
				val: "a:^[Describe=4.2:5",
			},
			wantErr: errors.Errorf("^[Describe must be valid regex expression for operation"),
		},
		{
			name: "when rate is not valid float number",
			fields: fields{
				value: map[string][]throttleConfig{},
			},
			args: args{
				val: "a:^Describe=4.x:5",
			},
			wantErr: errors.Errorf("4.x must be valid float number as rate for operations per second"),
		},
		{
			name: "when burst is not valid integer",
			fields: fields{
				value: map[string][]throttleConfig{},
			},
			args: args{
				val: "a:^Describe=4.2:5x",
			},
			wantErr: errors.Errorf("5x must be valid integer as burst for operations"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ServiceOperationsThrottleConfig{
				value: tt.fields.value,
			}
			err := c.Set(tt.args.val)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, *c)
			}
		})
	}
}

func TestServiceOperationsThrottleConfig_Type(t *testing.T) {
	c := &ServiceOperationsThrottleConfig{}
	got := c.Type()
	assert.Equal(t, "serviceOperationsThrottleConfig", got)
}
