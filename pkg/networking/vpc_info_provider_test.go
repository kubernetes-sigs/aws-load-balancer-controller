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

func Test_defaultVPCInfoProvider_FetchVPCInfo(t *testing.T) {
	type describeVpcsCall struct {
		req  *ec2sdk.DescribeVpcsInput
		resp *ec2sdk.DescribeVpcsOutput
		err  error
	}
	type fields struct {
		describeVpcsCalls []describeVpcsCall
	}
	type fetchVPCInfoCall struct {
		vpcID   string
		opts    []FetchVPCInfoOption
		want    VPCInfo
		wantErr error
	}
	tests := []struct {
		name              string
		fields            fields
		fetchVPCInfoCalls []fetchVPCInfoCall
	}{
		{
			name: "fetch single VPC twice with cache",
			fields: fields{
				describeVpcsCalls: []describeVpcsCall{
					{
						req: &ec2sdk.DescribeVpcsInput{
							VpcIds: awssdk.StringSlice([]string{"vpc-2f09a348"}),
						},
						resp: &ec2sdk.DescribeVpcsOutput{
							Vpcs: []*ec2sdk.Vpc{
								{
									VpcId: awssdk.String("vpc-2f09a348"),
									CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
										{
											CidrBlock: awssdk.String("192.168.0.0/16"),
											CidrBlockState: &ec2sdk.VpcCidrBlockState{
												State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					vpcID: "vpc-2f09a348",
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a348"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("192.168.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
						},
					},
				},
				{
					vpcID: "vpc-2f09a348",
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a348"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("192.168.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fetch single VPC twice without cache",
			fields: fields{
				describeVpcsCalls: []describeVpcsCall{
					{
						req: &ec2sdk.DescribeVpcsInput{
							VpcIds: awssdk.StringSlice([]string{"vpc-2f09a348"}),
						},
						resp: &ec2sdk.DescribeVpcsOutput{
							Vpcs: []*ec2sdk.Vpc{
								{
									VpcId: awssdk.String("vpc-2f09a348"),
									CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
										{
											CidrBlock: awssdk.String("192.168.0.0/16"),
											CidrBlockState: &ec2sdk.VpcCidrBlockState{
												State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
											},
										},
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeVpcsInput{
							VpcIds: awssdk.StringSlice([]string{"vpc-2f09a348"}),
						},
						resp: &ec2sdk.DescribeVpcsOutput{
							Vpcs: []*ec2sdk.Vpc{
								{
									VpcId: awssdk.String("vpc-2f09a348"),
									CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
										{
											CidrBlock: awssdk.String("192.168.0.0/16"),
											CidrBlockState: &ec2sdk.VpcCidrBlockState{
												State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
											},
										},
										{
											CidrBlock: awssdk.String("10.100.0.0/16"),
											CidrBlockState: &ec2sdk.VpcCidrBlockState{
												State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					vpcID: "vpc-2f09a348",
					opts:  []FetchVPCInfoOption{FetchVPCInfoWithoutCache()},
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a348"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("192.168.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
						},
					},
				},
				{
					vpcID: "vpc-2f09a348",
					opts:  []FetchVPCInfoOption{FetchVPCInfoWithoutCache()},
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a348"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("192.168.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
							{
								CidrBlock: awssdk.String("10.100.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fetch multiple VPC twice with cache",
			fields: fields{
				describeVpcsCalls: []describeVpcsCall{
					{
						req: &ec2sdk.DescribeVpcsInput{
							VpcIds: awssdk.StringSlice([]string{"vpc-2f09a348"}),
						},
						resp: &ec2sdk.DescribeVpcsOutput{
							Vpcs: []*ec2sdk.Vpc{
								{
									VpcId: awssdk.String("vpc-2f09a348"),
									CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
										{
											CidrBlock: awssdk.String("192.168.0.0/16"),
											CidrBlockState: &ec2sdk.VpcCidrBlockState{
												State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
											},
										},
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeVpcsInput{
							VpcIds: awssdk.StringSlice([]string{"vpc-2f09a842"}),
						},
						resp: &ec2sdk.DescribeVpcsOutput{
							Vpcs: []*ec2sdk.Vpc{
								{
									VpcId: awssdk.String("vpc-2f09a842"),
									CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
										{
											CidrBlock: awssdk.String("10.100.0.0/16"),
											CidrBlockState: &ec2sdk.VpcCidrBlockState{
												State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					vpcID: "vpc-2f09a348",
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a348"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("192.168.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
						},
					},
				},
				{
					vpcID: "vpc-2f09a842",
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a842"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("10.100.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
						},
					},
				},
				{
					vpcID: "vpc-2f09a348",
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a348"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("192.168.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
								},
							},
						},
					},
				},
				{
					vpcID: "vpc-2f09a842",
					want: VPCInfo{
						VpcId: awssdk.String("vpc-2f09a842"),
						CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
							{
								CidrBlock: awssdk.String("10.100.0.0/16"),
								CidrBlockState: &ec2sdk.VpcCidrBlockState{
									State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
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
			for _, call := range tt.fields.describeVpcsCalls {
				ec2Client.EXPECT().DescribeVpcsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			p := NewDefaultVPCInfoProvider(ec2Client, &log.NullLogger{})
			for _, call := range tt.fetchVPCInfoCalls {
				got, err := p.FetchVPCInfo(context.Background(), call.vpcID, call.opts...)
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

func Test_defaultVPCInfoProvider_fetchVPCInfoFromAWS(t *testing.T) {
	type describeVpcsCall struct {
		req  *ec2sdk.DescribeVpcsInput
		resp *ec2sdk.DescribeVpcsOutput
		err  error
	}
	type fields struct {
		describeVpcsCalls []describeVpcsCall
	}
	type args struct {
		vpcID string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    VPCInfo
		wantErr error
	}{
		{
			name: "describeVpcs succeeded",
			fields: fields{
				describeVpcsCalls: []describeVpcsCall{
					{
						req: &ec2sdk.DescribeVpcsInput{
							VpcIds: awssdk.StringSlice([]string{"vpc-2f09a348"}),
						},
						resp: &ec2sdk.DescribeVpcsOutput{
							Vpcs: []*ec2sdk.Vpc{
								{
									VpcId: awssdk.String("vpc-2f09a348"),
									CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
										{
											CidrBlock: awssdk.String("192.168.0.0/16"),
											CidrBlockState: &ec2sdk.VpcCidrBlockState{
												State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				vpcID: "vpc-2f09a348",
			},
			want: VPCInfo{
				VpcId: awssdk.String("vpc-2f09a348"),
				CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
					{
						CidrBlock: awssdk.String("192.168.0.0/16"),
						CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
						},
					},
				},
			},
		},
		{
			name: "describeVpcs failed",
			fields: fields{
				describeVpcsCalls: []describeVpcsCall{
					{
						req: &ec2sdk.DescribeVpcsInput{
							VpcIds: awssdk.StringSlice([]string{"vpc-2f09a348"}),
						},
						err: errors.New("some error happened"),
					},
				},
			},
			args: args{
				vpcID: "vpc-2f09a348",
			},
			wantErr: errors.New("some error happened"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeVpcsCalls {
				ec2Client.EXPECT().DescribeVpcsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			p := &defaultVPCInfoProvider{
				ec2Client: ec2Client,
				logger:    &log.NullLogger{},
			}
			got, err := p.fetchVPCInfoFromAWS(context.Background(), tt.args.vpcID)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestVPCInfo_AssociatedIPv4CIDRs(t *testing.T) {
	tests := []struct {
		name string
		vpc  VPCInfo
		want []string
	}{
		{
			name: "single associated CIDR",
			vpc: VPCInfo{
				VpcId: awssdk.String("vpc-2f09a348"),
				CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
					{
						CidrBlock: awssdk.String("192.168.0.0/16"),
						CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
						},
					},
				},
			},
			want: []string{"192.168.0.0/16"},
		},
		{
			name: "multiple CIDRs",
			vpc: VPCInfo{
				VpcId: awssdk.String("vpc-2f09a348"),
				CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
					{
						CidrBlock: awssdk.String("192.168.0.0/16"),
						CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
						},
					},
					{
						CidrBlock: awssdk.String("10.100.0.0/16"),
						CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeDisassociated),
						},
					},
					{
						CidrBlock: awssdk.String("172.16.0.0/16"),
						CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
						},
					},
				},
			},
			want: []string{"192.168.0.0/16", "172.16.0.0/16"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.vpc.AssociatedIPv4CIDRs()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVPCInfo_AssociatedIPv6CIDRs(t *testing.T) {
	tests := []struct {
		name string
		vpc  VPCInfo
		want []string
	}{
		{
			name: "single associated CIDR",
			vpc: VPCInfo{
				VpcId: awssdk.String("vpc-2f09a348"),
				Ipv6CidrBlockAssociationSet: []*ec2sdk.VpcIpv6CidrBlockAssociation{
					{
						Ipv6CidrBlock: awssdk.String("2600:1f14:f8c:2700::/56"),
						Ipv6CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
						},
					},
				},
			},
			want: []string{"2600:1f14:f8c:2700::/56"},
		},
		{
			name: "multiple CIDRs",
			vpc: VPCInfo{
				VpcId: awssdk.String("vpc-2f09a348"),
				Ipv6CidrBlockAssociationSet: []*ec2sdk.VpcIpv6CidrBlockAssociation{
					{
						Ipv6CidrBlock: awssdk.String("2600:1f14:f8c:2700::/56"),
						Ipv6CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
						},
					},
					{
						Ipv6CidrBlock: awssdk.String("2700:1f14:f8c:2700::/56"),
						Ipv6CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeDisassociated),
						},
					},
					{
						Ipv6CidrBlock: awssdk.String("2800:1f14:f8c:2700::/56"),
						Ipv6CidrBlockState: &ec2sdk.VpcCidrBlockState{
							State: awssdk.String(ec2sdk.VpcCidrBlockStateCodeAssociated),
						},
					},
				},
			},
			want: []string{"2600:1f14:f8c:2700::/56", "2800:1f14:f8c:2700::/56"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.vpc.AssociatedIPv6CIDRs()
			assert.Equal(t, tt.want, got)
		})
	}
}
