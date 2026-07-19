package services

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
)

func newCachedRoute53Client(zones []types.HostedZone) *route53Client {
	c := &route53Client{
		hostedZonesCache:    cache.NewExpiring(),
		hostedZonesCacheTTL: defaultHostedZonesCacheTTL,
	}
	c.hostedZonesCache.Set(hostedZonesCacheKey, zones, time.Hour)
	return c
}

func hostedZone(id, name string) types.HostedZone {
	return types.HostedZone{Id: awssdk.String(id), Name: awssdk.String(name)}
}

func privateHostedZone(id, name string) types.HostedZone {
	return types.HostedZone{
		Id:     awssdk.String(id),
		Name:   awssdk.String(name),
		Config: &types.HostedZoneConfig{PrivateZone: true},
	}
}

func TestGetHostedZoneID(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		zones  []types.HostedZone
		want   string
	}{
		{
			name:   "exact apex match",
			domain: "example.com",
			zones:  []types.HostedZone{hostedZone("Z_EXAMPLE", "example.com.")},
			want:   "Z_EXAMPLE",
		},
		{
			name:   "wildcard SAN two labels below the zone",
			domain: "*.app.sub.example.com",
			zones:  []types.HostedZone{hostedZone("Z_SUB", "sub.example.com.")},
			want:   "Z_SUB",
		},
		{
			name:   "longest suffix wins over parent zone",
			domain: "*.app.sub.example.com",
			zones: []types.HostedZone{
				hostedZone("Z_PARENT", "example.com."),
				hostedZone("Z_SUB", "sub.example.com."),
			},
			want: "Z_SUB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCachedRoute53Client(tt.zones)
			got, err := c.GetHostedZoneID(context.Background(), tt.domain)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, awssdk.ToString(got))
		})
	}
}

func TestGetPublicHostedZoneID(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		zones   []types.HostedZone
		want    string
		wantErr bool
	}{
		{
			name:   "split-horizon: public chosen even though private zone is more specific",
			domain: "app.sub.example.com",
			zones: []types.HostedZone{
				hostedZone("Z_PUBLIC", "example.com."),
				privateHostedZone("Z_PRIVATE", "sub.example.com."),
			},
			want: "Z_PUBLIC",
		},
		{
			name:   "only a private zone matches: fail fast",
			domain: "app.sub.example.com",
			zones: []types.HostedZone{
				privateHostedZone("Z_PRIVATE", "sub.example.com."),
			},
			wantErr: true,
		},
		{
			name:   "multiple public zones match: longest suffix wins",
			domain: "*.app.sub.example.com",
			zones: []types.HostedZone{
				hostedZone("Z_PARENT", "example.com."),
				hostedZone("Z_SUB", "sub.example.com."),
			},
			want: "Z_SUB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCachedRoute53Client(tt.zones)
			got, err := c.GetPublicHostedZoneID(context.Background(), tt.domain)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, awssdk.ToString(got))
		})
	}
}

// The unfiltered lookup (delete path) must still resolve private zones so legacy
// records written there can be cleaned up.
func TestGetHostedZoneID_UnfilteredReturnsPrivate(t *testing.T) {
	c := newCachedRoute53Client([]types.HostedZone{
		hostedZone("Z_PUBLIC", "example.com."),
		privateHostedZone("Z_PRIVATE", "sub.example.com."),
	})
	got, err := c.GetHostedZoneID(context.Background(), "app.sub.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "Z_PRIVATE", awssdk.ToString(got))
}
