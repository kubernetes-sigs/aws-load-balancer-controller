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
