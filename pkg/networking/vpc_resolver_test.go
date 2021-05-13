package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"testing"
)

func Test_defaultVPCResolver_ResolveCIDRs(t *testing.T) {
	type descriveVpcsCall struct {
		input  *ec2sdk.DescribeVpcsInput
		output *ec2sdk.DescribeVpcsOutput
		err    error
	}
	tests := []struct {
		name             string
		vpcID            string
		want             []string
		wantErr          error
		descriveVpcsCall descriveVpcsCall
	}{
		{
			name:    "vpc cidr discovery",
			vpcID:   "vpc-01xxx2",
			want:    []string{"192.160.0.0/16"},
			wantErr: nil,
			descriveVpcsCall: descriveVpcsCall{
				input: &ec2sdk.DescribeVpcsInput{
					VpcIds: []*string{awssdk.String("vpc-01xxx2")},
				},
				output: &ec2sdk.DescribeVpcsOutput{
					Vpcs: []*ec2sdk.Vpc{
						{
							CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
								{
									CidrBlock: awssdk.String("192.160.0.0/16"),
								},
							},
						},
					},
				},
			},
		},
		{
			name:    "unable to describe VPC",
			vpcID:   "vpc-01xxx3",
			wantErr: errors.Wrapf(errors.New("aws error"), "unable to describe VPC"),
			descriveVpcsCall: descriveVpcsCall{
				input: &ec2sdk.DescribeVpcsInput{
					VpcIds: []*string{awssdk.String("vpc-01xxx3")},
				},
				err: errors.New("aws error"),
			},
		},
		{
			name:    "unable to find matching VPC",
			vpcID:   "vpc-01xxx4",
			wantErr: errors.New("unable to find matching VPC \"vpc-01xxx4\""),
			descriveVpcsCall: descriveVpcsCall{
				input: &ec2sdk.DescribeVpcsInput{
					VpcIds: []*string{awssdk.String("vpc-01xxx4")},
				},
				output: &ec2sdk.DescribeVpcsOutput{},
			},
		},
		{
			name:    "multiple CIDRs",
			vpcID:   "vpc-01xxx2",
			want:    []string{"192.160.0.0/16", "100.64.0.0/16", "100.65.0.0/16", "100.66.0.0/24"},
			wantErr: nil,
			descriveVpcsCall: descriveVpcsCall{
				input: &ec2sdk.DescribeVpcsInput{
					VpcIds: []*string{awssdk.String("vpc-01xxx2")},
				},
				output: &ec2sdk.DescribeVpcsOutput{
					Vpcs: []*ec2sdk.Vpc{
						{
							CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
								{
									CidrBlock: awssdk.String("192.160.0.0/16"),
								},
								{
									CidrBlock: awssdk.String("100.64.0.0/16"),
								},
								{
									CidrBlock: awssdk.String("100.65.0.0/16"),
								},
								{
									CidrBlock: awssdk.String("100.66.0.0/24"),
								},
							},
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
			ec2Client.EXPECT().DescribeVpcsWithContext(gomock.Any(), tt.descriveVpcsCall.input).Return(
				tt.descriveVpcsCall.output, tt.descriveVpcsCall.err)
			vpcResolver := &defaultVPCResolver{
				ec2Client: ec2Client,
				vpcID:     tt.vpcID,
			}
			got, err := vpcResolver.ResolveCIDRs(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

		})
	}
}
