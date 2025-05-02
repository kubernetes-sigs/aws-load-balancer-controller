package model

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func TestLoadBalancerBuilderImpl_BuildLoadBalancerName(t *testing.T) {
	tests := []struct {
		name        string
		lbConf      elbv2gw.LoadBalancerConfiguration
		gw          *gwv1.Gateway
		scheme      elbv2model.LoadBalancerScheme
		clusterName string
		want        string
		wantErr     bool
	}{
		{
			name: "specify load balancer name in load balancer configuration",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: aws.String("my-example-lb"),
				},
			},
			gw:      &gwv1.Gateway{},
			scheme:  elbv2model.LoadBalancerSchemeInternal,
			want:    "my-example-lb",
			wantErr: false,
		},
		{
			name: "generated name with valid characters",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-gateway",
					UID:       types.UID("123e4567-e89b-12d3-a456-426614174000"),
				},
			},
			scheme:      elbv2model.LoadBalancerSchemeInternal,
			clusterName: "test-cluster",
			want:        "k8s-test-name-test-gate-0123456789",
			wantErr:     false,
		},
		{
			name: "generated name with invalid characters",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test@namespace",
					Name:      "test#gateway",
					UID:       types.UID("123e4567-e89b-12d3-a456-426614174000"),
				},
			},
			scheme:      elbv2model.LoadBalancerSchemeInternal,
			clusterName: "test-cluster",
			want:        "k8s-testnames-testgatew-0123456789",
			wantErr:     false,
		},
		{
			name: "provide long namespace and name",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "very-long-namespace-that-exceeds-limit",
					Name:      "very-long-gateway-name-that-exceeds-limit",
					UID:       types.UID("123e4567-e89b-12d3-a456-426614174000"),
				},
			},
			scheme:      elbv2model.LoadBalancerSchemeInternal,
			clusterName: "test-cluster",
			want:        "k8s-very-long-very-long-0123456789",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbModelBuilder := &loadBalancerBuilderImpl{
				clusterName: tt.clusterName,
			}

			got, err := lbModelBuilder.buildLoadBalancerName(tt.lbConf, tt.gw, tt.scheme)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.lbConf.Spec.LoadBalancerName != nil {
				assert.Equal(t, tt.want, got)
			} else {
				assert.Regexp(t, "^k8s-[a-zA-Z0-9-]+-[a-zA-Z0-9-]+-[a-zA-Z0-9]+$", got)
				assert.LessOrEqual(t, len(got), 32) // AWS LB name length limit
				parts := strings.Split(got, "-")
				assert.Equal(t, 4, len(parts))
				assert.LessOrEqual(t, len(parts[1]), 8) // namespace part
				assert.LessOrEqual(t, len(parts[2]), 8) // name part
				assert.Equal(t, 10, len(parts[3]))      // uuid part
			}
		})
	}
}

func TestLoadBalancerBuilderImpl_BuildLoadBalancerAttributes(t *testing.T) {
	tests := []struct {
		name   string
		lbConf elbv2gw.LoadBalancerConfiguration
		want   []elbv2model.LoadBalancerAttribute
	}{
		{
			name: "provide single attribute",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "deletion_protection.enabled",
							Value: "true",
						},
					},
				},
			},
			want: []elbv2model.LoadBalancerAttribute{
				{
					Key:   "deletion_protection.enabled",
					Value: "true",
				},
			},
		},
		{
			name: "provide multiple attributes",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "deletion_protection.enabled",
							Value: "true",
						},
						{
							Key:   "idle_timeout.timeout_seconds",
							Value: "60",
						},
					},
				},
			},
			want: []elbv2model.LoadBalancerAttribute{
				{
					Key:   "deletion_protection.enabled",
					Value: "true",
				},
				{
					Key:   "idle_timeout.timeout_seconds",
					Value: "60",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbModelBuilder := &loadBalancerBuilderImpl{}
			got := lbModelBuilder.buildLoadBalancerAttributes(tt.lbConf)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadBalancerBuilderImpl_BuildLoadBalancerMinimumCapacity(t *testing.T) {
	tests := []struct {
		name   string
		lbConf elbv2gw.LoadBalancerConfiguration
		want   *elbv2model.MinimumLoadBalancerCapacity
	}{
		{
			name: "MinimumLoadBalancerCapacity is nil",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					MinimumLoadBalancerCapacity: nil,
				},
			},
			want: nil,
		},
		{
			name: "MinimumLoadBalancerCapacity with a valid value",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					MinimumLoadBalancerCapacity: &elbv2gw.MinimumLoadBalancerCapacity{
						CapacityUnits: 100,
					},
				},
			},
			want: &elbv2model.MinimumLoadBalancerCapacity{
				CapacityUnits: 100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbModelBuilder := &loadBalancerBuilderImpl{}
			got := lbModelBuilder.buildLoadBalancerMinimumCapacity(tt.lbConf)
			assert.Equal(t, tt.want, got)
		})
	}
}
