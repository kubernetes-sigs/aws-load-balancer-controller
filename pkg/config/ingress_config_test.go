package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IngressConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     IngressConfig
		wantErr bool
	}{
		{
			name: "default config is valid",
			cfg: IngressConfig{
				Route53ValidationRecordRoutingPolicy: defaultRoute53ValidationRecordRoutingPolicy,
				Route53ValidationRecordWeight:        defaultRoute53ValidationRecordWeight,
			},
			wantErr: false,
		},
		{
			name: "explicit simple policy is valid regardless of weight",
			cfg: IngressConfig{
				Route53ValidationRecordRoutingPolicy: Route53RoutingPolicySimple,
				Route53ValidationRecordWeight:        0,
			},
			wantErr: false,
		},
		{
			name: "weighted policy with positive weight is valid",
			cfg: IngressConfig{
				Route53ValidationRecordRoutingPolicy: Route53RoutingPolicyWeighted,
				Route53ValidationRecordWeight:        50,
			},
			wantErr: false,
		},
		{
			name: "weighted policy with zero weight is invalid",
			cfg: IngressConfig{
				Route53ValidationRecordRoutingPolicy: Route53RoutingPolicyWeighted,
				Route53ValidationRecordWeight:        0,
			},
			wantErr: true,
		},
		{
			name: "weighted policy with negative weight is invalid",
			cfg: IngressConfig{
				Route53ValidationRecordRoutingPolicy: Route53RoutingPolicyWeighted,
				Route53ValidationRecordWeight:        -1,
			},
			wantErr: true,
		},
		{
			name: "unknown routing policy is invalid",
			cfg: IngressConfig{
				Route53ValidationRecordRoutingPolicy: "bogus",
				Route53ValidationRecordWeight:        100,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
