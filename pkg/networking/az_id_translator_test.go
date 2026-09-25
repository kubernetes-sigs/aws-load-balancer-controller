package networking

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultAZIDTranslator_TranslateAZName(t *testing.T) {
	type describeAvailabilityZonesCall struct {
		zones []ec2types.AvailabilityZone
		err   error
	}
	type translateAZNameCall struct {
		srcZoneName   string
		assumeRoleArn string
		want          *string
		wantErr       error
	}
	srcZones := []ec2types.AvailabilityZone{
		{
			ZoneName: awssdk.String("us-east-1a"),
			ZoneId:   awssdk.String("use1-az4"),
		},
		{
			ZoneName: awssdk.String("us-east-1b"),
			ZoneId:   awssdk.String("use1-az6"),
		},
	}
	tests := []struct {
		name                 string
		srcDescribeAZCalls   []describeAvailabilityZonesCall
		dstDescribeAZCalls   []describeAvailabilityZonesCall
		translateAZNameCalls []translateAZNameCall
	}{
		{
			name:               "translates a zone name across accounts",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{{zones: srcZones}},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					zones: []ec2types.AvailabilityZone{
						{
							ZoneName: awssdk.String("us-east-1c"),
							ZoneId:   awssdk.String("use1-az4"),
						},
					},
				},
			},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					want:          awssdk.String("us-east-1c"),
				},
			},
		},
		{
			// A single describe call per account serves every zone of that account.
			name:               "repeated translations hit the cache",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{{zones: srcZones}},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					zones: []ec2types.AvailabilityZone{
						{
							ZoneName: awssdk.String("us-east-1c"),
							ZoneId:   awssdk.String("use1-az4"),
						},
						{
							ZoneName: awssdk.String("us-east-1a"),
							ZoneId:   awssdk.String("use1-az6"),
						},
					},
				},
			},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					want:          awssdk.String("us-east-1c"),
				},
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					want:          awssdk.String("us-east-1c"),
				},
				{
					srcZoneName:   "us-east-1b",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					want:          awssdk.String("us-east-1a"),
				},
			},
		},
		{
			name:               "two destination accounts do not share cache entries",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{{zones: srcZones}},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					zones: []ec2types.AvailabilityZone{
						{
							ZoneName: awssdk.String("us-east-1c"),
							ZoneId:   awssdk.String("use1-az4"),
						},
					},
				},
				{
					zones: []ec2types.AvailabilityZone{
						{
							ZoneName: awssdk.String("us-east-1f"),
							ZoneId:   awssdk.String("use1-az4"),
						},
					},
				},
			},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/RoleA",
					want:          awssdk.String("us-east-1c"),
				},
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::210987654321:role/RoleB",
					want:          awssdk.String("us-east-1f"),
				},
			},
		},
		{
			// The unfiltered describe call is authoritative, so an unknown zone name never causes
			// another call for it.
			name:               "unknown source zone name returns nil without another describe call",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{{zones: srcZones}},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1e",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					want:          nil,
				},
				{
					srcZoneName:   "us-east-1e",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					want:          nil,
				},
			},
		},
		{
			name:               "zone ID unknown in the destination account returns nil",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{{zones: srcZones}},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					zones: []ec2types.AvailabilityZone{
						{
							ZoneName: awssdk.String("us-east-1c"),
							ZoneId:   awssdk.String("use1-az6"),
						},
					},
				},
			},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					want:          nil,
				},
			},
		},
		{
			name:               "source lookup error is returned",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{{err: errors.New("UnauthorizedOperation")}},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					wantErr:       errors.New("UnauthorizedOperation"),
				},
			},
		},
		{
			name:               "destination lookup error is returned",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{{zones: srcZones}},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{{err: errors.New("AccessDenied")}},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					wantErr:       errors.New("AccessDenied"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			expectDescribeAZCalls := func(ec2Client *services.MockEC2, calls []describeAvailabilityZonesCall) {
				for _, call := range calls {
					var output *ec2sdk.DescribeAvailabilityZonesOutput
					if call.err == nil {
						output = &ec2sdk.DescribeAvailabilityZonesOutput{AvailabilityZones: call.zones}
					}
					ec2Client.EXPECT().DescribeAvailabilityZonesWithContext(gomock.Any(), &ec2sdk.DescribeAvailabilityZonesInput{}).Return(output, call.err)
				}
			}

			srcEC2 := services.NewMockEC2(ctrl)
			expectDescribeAZCalls(srcEC2, tt.srcDescribeAZCalls)
			dstEC2 := services.NewMockEC2(ctrl)
			expectDescribeAZCalls(dstEC2, tt.dstDescribeAZCalls)
			srcEC2.EXPECT().AssumeRole(gomock.Any(), gomock.Any(), gomock.Any()).Return(dstEC2, nil).AnyTimes()

			translator := NewDefaultAZIDTranslator(srcEC2, logr.New(&log.NullLogSink{}))

			for _, call := range tt.translateAZNameCalls {
				got, err := translator.TranslateAZName(context.Background(), call.assumeRoleArn, "", call.srcZoneName)
				if call.wantErr != nil {
					assert.EqualError(t, err, call.wantErr.Error())
					continue
				}
				assert.NoError(t, err)
				assert.Equal(t, awssdk.ToString(call.want), awssdk.ToString(got))
			}
		})
	}
}
