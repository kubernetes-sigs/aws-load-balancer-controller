package services

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
)

type stubHostedZoneLister struct {
	pages   map[string]*route53.ListHostedZonesOutput
	err     error
	markers []*string
}

func (s *stubHostedZoneLister) ListHostedZones(_ context.Context, params *route53.ListHostedZonesInput, _ ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	s.markers = append(s.markers, params.Marker)
	if s.err != nil {
		return nil, s.err
	}
	key := ""
	if params.Marker != nil {
		key = *params.Marker
	}
	out, ok := s.pages[key]
	if !ok {
		return nil, errors.New("unexpected marker: " + key)
	}
	return out, nil
}

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

func TestListAllHostedZones(t *testing.T) {
	page2Marker := "page-2"
	page1Zones := []types.HostedZone{
		hostedZone("Z1", "one.example.com."),
		hostedZone("Z2", "two.example.com."),
	}
	page2Zones := []types.HostedZone{
		hostedZone("Z3", "three.example.com."),
	}
	singlePageZones := []types.HostedZone{
		hostedZone("Z1", "one.example.com."),
	}
	apiErr := errors.New("list hosted zones failed")

	tests := []struct {
		name        string
		pages       map[string]*route53.ListHostedZonesOutput
		err         error
		wantZones   []types.HostedZone
		wantErr     error
		wantMarkers []*string
	}{
		{
			name: "two pages",
			pages: map[string]*route53.ListHostedZonesOutput{
				"": {
					HostedZones: page1Zones,
					IsTruncated: true,
					NextMarker:  awssdk.String(page2Marker),
				},
				page2Marker: {
					HostedZones: page2Zones,
					IsTruncated: false,
				},
			},
			wantZones: []types.HostedZone{
				hostedZone("Z1", "one.example.com."),
				hostedZone("Z2", "two.example.com."),
				hostedZone("Z3", "three.example.com."),
			},
			wantMarkers: []*string{nil, awssdk.String(page2Marker)},
		},
		{
			name: "single page",
			pages: map[string]*route53.ListHostedZonesOutput{
				"": {
					HostedZones: singlePageZones,
					IsTruncated: false,
				},
			},
			wantZones: singlePageZones,
		},
		{
			name:    "API error on the first call",
			err:     apiErr,
			wantErr: apiErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &stubHostedZoneLister{pages: tt.pages, err: tt.err}
			got, err := listAllHostedZones(context.Background(), lister)
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantZones, got)
			if tt.wantMarkers != nil {
				assert.Equal(t, tt.wantMarkers, lister.markers)
			}
		})
	}
}
