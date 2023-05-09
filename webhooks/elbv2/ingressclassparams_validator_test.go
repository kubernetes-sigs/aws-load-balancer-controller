package elbv2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
)

func Test_ingressClassParamsValidator_ValidateCreate(t *testing.T) {
	tests := []struct {
		name    string
		obj     *elbv2api.IngressClassParams
		wantErr string
	}{
		{
			name: "empty",
			obj:  &elbv2api.IngressClassParams{},
		},
		{
			name: "inboundCIDRs is valid CIDR list",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					InboundCIDRs: []string{
						"10.0.0.0/8",
						"2001:DB8::/32",
					},
				},
			},
		},
		{
			name: "inboundCIDRs IPv4 no length",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					InboundCIDRs: []string{
						"192.168.0.1",
					},
				},
			},
			wantErr: "spec.inboundCIDRs[0]: Invalid value: \"192.168.0.1\": Could not be parsed as a CIDR (did you mean \"192.168.0.1/32\")",
		},
		{
			name: "inboundCIDRs IPv6 no length",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					InboundCIDRs: []string{
						"2001:DB8::",
					},
				},
			},
			wantErr: "spec.inboundCIDRs[0]: Invalid value: \"2001:DB8::\": Could not be parsed as a CIDR (did you mean \"2001:DB8::/64\")",
		},
		{
			name: "inboundCIDRs bits outside prefix",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					InboundCIDRs: []string{
						"10.128.0.0/8",
					},
				},
			},
			wantErr: "spec.inboundCIDRs[0]: Invalid value: \"10.128.0.0/8\": Network contains bits outside prefix (did you mean \"10.0.0.0/8\")",
		},
		{
			name: "inboundCIDRs empty string",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					InboundCIDRs: []string{
						"",
					},
				},
			},
			wantErr: "spec.inboundCIDRs[0]: Invalid value: \"\": Could not be parsed as a CIDR",
		},
		{
			name: "inboundCIDRs domain",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					InboundCIDRs: []string{
						"invalid.example.com",
					},
				},
			},
			wantErr: "spec.inboundCIDRs[0]: Invalid value: \"invalid.example.com\": Could not be parsed as a CIDR",
		},
		{
			name: "subnet is valid ID list",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					Subnets: &elbv2api.SubnetSelector{
						IDs: []elbv2api.SubnetID{"subnet-1", "subnet-2"},
					},
				},
			},
		},
		{
			name: "subnet is valid tag list",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					Subnets: &elbv2api.SubnetSelector{
						Tags: map[string][]string{
							"key": {"value1", "value2"},
						},
					},
				},
			},
		},
		{
			name: "subnet selector empty",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					Subnets: &elbv2api.SubnetSelector{},
				},
			},
			wantErr: "spec.subnets: Required value: must have either `ids` or `tags`",
		},
		{
			name: "subnet selector with both id and tag",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					Subnets: &elbv2api.SubnetSelector{
						IDs: []elbv2api.SubnetID{"subnet-1", "subnet-2"},
						Tags: map[string][]string{
							"Name": {"named-subnet"},
						},
					},
				},
			},
			wantErr: "spec.subnets.tags: Forbidden: may not have both `ids` and `tags` set",
		},
		{
			name: "subnet duplicate id",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					Subnets: &elbv2api.SubnetSelector{
						IDs: []elbv2api.SubnetID{"subnet-1", "subnet-2", "subnet-1"},
					},
				},
			},
			wantErr: "spec.subnets.ids[2]: Duplicate value: \"subnet-1\"",
		},
		{
			name: "subnet duplicate tag value",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					Subnets: &elbv2api.SubnetSelector{
						Tags: map[string][]string{
							"Name":  {"name1"},
							"Other": {"other1", "other2", "other1"},
						},
					},
				},
			},
			wantErr: "spec.subnets.tags[Other][2]: Duplicate value: \"other1\"",
		},
		{
			name: "subnet empty tags map",
			obj: &elbv2api.IngressClassParams{
				Spec: elbv2api.IngressClassParamsSpec{
					Subnets: &elbv2api.SubnetSelector{
						Tags: map[string][]string{},
					},
				},
			},
			wantErr: "spec.subnets.tags: Required value: must have at least one tag key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &ingressClassParamsValidator{}
			t.Run("create", func(t *testing.T) {
				err := v.ValidateCreate(context.Background(), tt.obj)
				if tt.wantErr != "" {
					assert.EqualError(t, err, tt.wantErr)
				} else {
					assert.NoError(t, err)
				}
			})
			t.Run("update", func(t *testing.T) {
				err := v.ValidateUpdate(context.Background(), tt.obj, &elbv2api.IngressClassParams{})
				if tt.wantErr != "" {
					assert.EqualError(t, err, tt.wantErr)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}
