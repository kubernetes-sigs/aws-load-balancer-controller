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
		input  *ec2sdk.DescribeAvailabilityZonesInput
		output *ec2sdk.DescribeAvailabilityZonesOutput
		err    error
	}
	type translateAZNameCall struct {
		srcZoneName   string
		assumeRoleArn string
		want          *string
		wantErr       error
	}
	tests := []struct {
		name                 string
		srcDescribeAZCalls   []describeAvailabilityZonesCall
		dstDescribeAZCalls   []describeAvailabilityZonesCall
		translateAZNameCalls []translateAZNameCall
	}{
		{
			name: "translates a zone name across accounts",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneNames: []string{"us-east-1a"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1a"),
								ZoneId:   awssdk.String("use1-az4"),
							},
						},
					},
				},
			},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneIds: []string{"use1-az4"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1c"),
								ZoneId:   awssdk.String("use1-az4"),
							},
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
			name: "repeated translations hit the cache",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneNames: []string{"us-east-1a"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1a"),
								ZoneId:   awssdk.String("use1-az4"),
							},
						},
					},
				},
			},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneIds: []string{"use1-az4"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1c"),
								ZoneId:   awssdk.String("use1-az4"),
							},
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
			},
		},
		{
			name: "two destination accounts do not share cache entries",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneNames: []string{"us-east-1a"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1a"),
								ZoneId:   awssdk.String("use1-az4"),
							},
						},
					},
				},
			},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneIds: []string{"use1-az4"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1c"),
								ZoneId:   awssdk.String("use1-az4"),
							},
						},
					},
				},
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneIds: []string{"use1-az4"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1f"),
								ZoneId:   awssdk.String("use1-az4"),
							},
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
			name: "unknown source zone name returns nil",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneNames: []string{"us-east-1a"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{},
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
			name: "zone ID unknown in the destination account returns nil",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneNames: []string{"us-east-1a"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1a"),
								ZoneId:   awssdk.String("use1-az4"),
							},
						},
					},
				},
			},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneIds: []string{"use1-az4"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{},
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
			name: "source lookup error is returned",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneNames: []string{"us-east-1a"},
					},
					err: errors.New("UnauthorizedOperation"),
				},
			},
			translateAZNameCalls: []translateAZNameCall{
				{
					srcZoneName:   "us-east-1a",
					assumeRoleArn: "arn:aws:iam::123456789012:role/MyRole",
					wantErr:       errors.New("UnauthorizedOperation"),
				},
			},
		},
		{
			name: "destination lookup error is returned",
			srcDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneNames: []string{"us-east-1a"},
					},
					output: &ec2sdk.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName: awssdk.String("us-east-1a"),
								ZoneId:   awssdk.String("use1-az4"),
							},
						},
					},
				},
			},
			dstDescribeAZCalls: []describeAvailabilityZonesCall{
				{
					input: &ec2sdk.DescribeAvailabilityZonesInput{
						ZoneIds: []string{"use1-az4"},
					},
					err: errors.New("AccessDenied"),
				},
			},
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

			srcEC2 := services.NewMockEC2(ctrl)
			for _, call := range tt.srcDescribeAZCalls {
				srcEC2.EXPECT().DescribeAvailabilityZonesWithContext(gomock.Any(), call.input).Return(call.output, call.err)
			}
			dstEC2 := services.NewMockEC2(ctrl)
			for _, call := range tt.dstDescribeAZCalls {
				dstEC2.EXPECT().DescribeAvailabilityZonesWithContext(gomock.Any(), call.input).Return(call.output, call.err)
			}
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
