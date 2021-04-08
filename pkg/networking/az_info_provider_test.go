package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultAZInfoProvider_FetchAZInfos(t *testing.T) {
	type describeAvailabilityZonesCall struct {
		input  *ec2sdk.DescribeAvailabilityZonesInput
		output *ec2sdk.DescribeAvailabilityZonesOutput
		err    error
	}

	type fields struct {
		describeAvailabilityZonesCalls []describeAvailabilityZonesCall
	}
	type args struct {
		availabilityZoneIDs []string
	}
	type fetchAZInfoCall struct {
		args    args
		want    map[string]ec2sdk.AvailabilityZone
		wantErr error
	}
	tests := []struct {
		name             string
		fields           fields
		fetchAZInfoCalls []fetchAZInfoCall
	}{
		{
			name: "fetch AZInfo for two AZ sequentially",
			fields: fields{
				describeAvailabilityZonesCalls: []describeAvailabilityZonesCall{
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az1"}),
						},
						output: &ec2sdk.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2sdk.AvailabilityZone{
								{
									ZoneId:   awssdk.String("usw2-az1"),
									ZoneType: awssdk.String("availability-zone"),
								},
							},
						},
					},
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az2"}),
						},
						output: &ec2sdk.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2sdk.AvailabilityZone{
								{
									ZoneId:   awssdk.String("usw2-az2"),
									ZoneType: awssdk.String("availability-zone"),
								},
							},
						},
					},
				},
			},
			fetchAZInfoCalls: []fetchAZInfoCall{
				{
					args: args{
						availabilityZoneIDs: []string{"usw2-az1"},
					},
					want: map[string]ec2sdk.AvailabilityZone{
						"usw2-az1": {
							ZoneId:   awssdk.String("usw2-az1"),
							ZoneType: awssdk.String("availability-zone"),
						},
					},
				},
				{
					args: args{
						availabilityZoneIDs: []string{"usw2-az2"},
					},
					want: map[string]ec2sdk.AvailabilityZone{
						"usw2-az2": {
							ZoneId:   awssdk.String("usw2-az2"),
							ZoneType: awssdk.String("availability-zone"),
						},
					},
				},
			},
		},
		{
			name: "fetch AZInfo for two AZ in batch",
			fields: fields{
				describeAvailabilityZonesCalls: []describeAvailabilityZonesCall{
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az1", "usw2-az2"}),
						},
						output: &ec2sdk.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2sdk.AvailabilityZone{
								{
									ZoneId:   awssdk.String("usw2-az1"),
									ZoneType: awssdk.String("availability-zone"),
								},
								{
									ZoneId:   awssdk.String("usw2-az2"),
									ZoneType: awssdk.String("availability-zone"),
								},
							},
						},
					},
				},
			},
			fetchAZInfoCalls: []fetchAZInfoCall{
				{
					args: args{
						availabilityZoneIDs: []string{"usw2-az1", "usw2-az2"},
					},
					want: map[string]ec2sdk.AvailabilityZone{
						"usw2-az1": {
							ZoneId:   awssdk.String("usw2-az1"),
							ZoneType: awssdk.String("availability-zone"),
						},
						"usw2-az2": {
							ZoneId:   awssdk.String("usw2-az2"),
							ZoneType: awssdk.String("availability-zone"),
						},
					},
				},
			},
		},
		{
			name: "fetch AZInfo for one AZ first then two AZ in batch",
			fields: fields{
				describeAvailabilityZonesCalls: []describeAvailabilityZonesCall{
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az1"}),
						},
						output: &ec2sdk.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2sdk.AvailabilityZone{
								{
									ZoneId:   awssdk.String("usw2-az1"),
									ZoneType: awssdk.String("availability-zone"),
								},
							},
						},
					},
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az2"}),
						},
						output: &ec2sdk.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2sdk.AvailabilityZone{
								{
									ZoneId:   awssdk.String("usw2-az2"),
									ZoneType: awssdk.String("availability-zone"),
								},
							},
						},
					},
				},
			},
			fetchAZInfoCalls: []fetchAZInfoCall{
				{
					args: args{
						availabilityZoneIDs: []string{"usw2-az1"},
					},
					want: map[string]ec2sdk.AvailabilityZone{
						"usw2-az1": {
							ZoneId:   awssdk.String("usw2-az1"),
							ZoneType: awssdk.String("availability-zone"),
						},
					},
				},
				{
					args: args{
						availabilityZoneIDs: []string{"usw2-az1", "usw2-az2"},
					},
					want: map[string]ec2sdk.AvailabilityZone{
						"usw2-az1": {
							ZoneId:   awssdk.String("usw2-az1"),
							ZoneType: awssdk.String("availability-zone"),
						},
						"usw2-az2": {
							ZoneId:   awssdk.String("usw2-az2"),
							ZoneType: awssdk.String("availability-zone"),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeAvailabilityZonesCalls {
				ec2Client.EXPECT().DescribeAvailabilityZonesWithContext(gomock.Any(), call.input).Return(call.output, call.err)
			}

			p := NewDefaultAZInfoProvider(ec2Client, &log.NullLogger{})

			for _, call := range tt.fetchAZInfoCalls {
				got, err := p.FetchAZInfos(context.Background(), call.args.availabilityZoneIDs)
				if call.wantErr != nil {
					assert.EqualError(t, err, call.wantErr.Error())
				} else {
					assert.NoError(t, err)
					assert.Equal(t, call.want, got)
				}
			}
		})
	}
}

func Test_defaultAZInfoProvider_fetchAZInfosFromAWS(t *testing.T) {
	type describeAvailabilityZonesCall struct {
		input  *ec2sdk.DescribeAvailabilityZonesInput
		output *ec2sdk.DescribeAvailabilityZonesOutput
		err    error
	}
	type fields struct {
		describeAvailabilityZonesCalls []describeAvailabilityZonesCall
	}
	type args struct {
		availabilityZoneIDs []string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]ec2sdk.AvailabilityZone
		wantErr error
	}{
		{
			name: "successfully fetched 1 AZ's info",
			fields: fields{
				describeAvailabilityZonesCalls: []describeAvailabilityZonesCall{
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az1"}),
						},
						output: &ec2sdk.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2sdk.AvailabilityZone{
								{
									ZoneId:   awssdk.String("usw2-az1"),
									ZoneType: awssdk.String("availability-zone"),
								},
							},
						},
					},
				},
			},
			args: args{
				availabilityZoneIDs: []string{"usw2-az1"},
			},
			want: map[string]ec2sdk.AvailabilityZone{
				"usw2-az1": {
					ZoneId:   awssdk.String("usw2-az1"),
					ZoneType: awssdk.String("availability-zone"),
				},
			},
		},
		{
			name: "successfully fetched 2 AZ's info",
			fields: fields{
				describeAvailabilityZonesCalls: []describeAvailabilityZonesCall{
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az1", "usw2-az2"}),
						},
						output: &ec2sdk.DescribeAvailabilityZonesOutput{
							AvailabilityZones: []*ec2sdk.AvailabilityZone{
								{
									ZoneId:   awssdk.String("usw2-az1"),
									ZoneType: awssdk.String("availability-zone"),
								},
								{
									ZoneId:   awssdk.String("usw2-az2"),
									ZoneType: awssdk.String("availability-zone"),
								},
							},
						},
					},
				},
			},
			args: args{
				availabilityZoneIDs: []string{"usw2-az1", "usw2-az2"},
			},
			want: map[string]ec2sdk.AvailabilityZone{
				"usw2-az1": {
					ZoneId:   awssdk.String("usw2-az1"),
					ZoneType: awssdk.String("availability-zone"),
				},
				"usw2-az2": {
					ZoneId:   awssdk.String("usw2-az2"),
					ZoneType: awssdk.String("availability-zone"),
				},
			},
		},
		{
			name: "failed fetched 2 AZ's info",
			fields: fields{
				describeAvailabilityZonesCalls: []describeAvailabilityZonesCall{
					{
						input: &ec2sdk.DescribeAvailabilityZonesInput{
							ZoneIds: awssdk.StringSlice([]string{"usw2-az1", "wrong-az-id"}),
						},
						output: nil,
						err:    errors.New("Invalid availability zone-id: wrong-az-id"),
					},
				},
			},
			args: args{
				availabilityZoneIDs: []string{"usw2-az1", "wrong-az-id"},
			},
			wantErr: errors.New("Invalid availability zone-id: wrong-az-id"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeAvailabilityZonesCalls {
				ec2Client.EXPECT().DescribeAvailabilityZonesWithContext(gomock.Any(), call.input).Return(call.output, call.err)
			}

			p := &defaultAZInfoProvider{
				ec2Client: ec2Client,
			}
			got, err := p.fetchAZInfosFromAWS(context.Background(), tt.args.availabilityZoneIDs)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_computeAZIDsWithoutAZInfo(t *testing.T) {
	type args struct {
		availabilityZoneIDs []string
		azInfoByAZID        map[string]ec2sdk.AvailabilityZone
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "all AZs have AZInfo",
			args: args{
				availabilityZoneIDs: []string{"usw2-az1", "usw2-az2"},
				azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
					"usw2-az1": {},
					"usw2-az2": {},
				},
			},
			want: []string{},
		},
		{
			name: "some AZ don't have AZInfo - non-empty azInfoByAZID",
			args: args{
				availabilityZoneIDs: []string{"usw2-az1", "usw2-az2"},
				azInfoByAZID: map[string]ec2sdk.AvailabilityZone{
					"usw2-az1": {},
				},
			},
			want: []string{"usw2-az2"},
		},
		{
			name: "some AZ don't have AZInfo - empty azInfoByAZID",
			args: args{
				availabilityZoneIDs: []string{"usw2-az1", "usw2-az2"},
				azInfoByAZID:        nil,
			},
			want: []string{"usw2-az1", "usw2-az2"},
		},
		{
			name: "empty availabilityZoneIDs and azInfoByAZID",
			args: args{
				availabilityZoneIDs: nil,
				azInfoByAZID:        nil,
			},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeAZIDsWithoutAZInfo(tt.args.availabilityZoneIDs, tt.args.azInfoByAZID)
			assert.Equal(t, tt.want, got)
		})
	}
}
