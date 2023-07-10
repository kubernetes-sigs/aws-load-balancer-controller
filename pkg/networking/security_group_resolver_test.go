package networking

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

func Test_defaultSecurityGroupResolver_ResolveViaNameOrID(t *testing.T) {
	type describeSecurityGroupsAsListCall struct {
		req  *ec2sdk.DescribeSecurityGroupsInput
		resp []*ec2sdk.SecurityGroup
		err  error
	}
	type args struct {
		nameOrIDs       []string
		describeSGCalls []describeSecurityGroupsAsListCall
	}
	defaultVPCID := "vpc-xxyy"
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "empty input",
		},
		{
			name: "group ids",
			args: args{
				nameOrIDs: []string{
					"sg-xx1",
					"sg-xx2",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: awssdk.StringSlice([]string{"sg-xx1", "sg-xx2"}),
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-xx1"),
							},
							{
								GroupId: awssdk.String("sg-xx2"),
							},
						},
					},
				},
			},
			want: []string{
				"sg-xx1",
				"sg-xx2",
			},
		},
		{
			name: "group names",
			args: args{
				nameOrIDs: []string{
					"sg group one",
					"sg group two",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: awssdk.StringSlice([]string{
										"sg group one",
										"sg group two",
									}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{defaultVPCID}),
								},
							},
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-0912f63b"),
							},
							{
								GroupId: awssdk.String("sg-08982de7"),
							},
						},
					},
				},
			},
			want: []string{
				"sg-08982de7",
				"sg-0912f63b",
			},
		},
		{
			name: "mixed group name and id",
			args: args{
				nameOrIDs: []string{
					"sg group one",
					"sg-id1",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: awssdk.StringSlice([]string{
										"sg group one",
									}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{defaultVPCID}),
								},
							},
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-0912f63b"),
							},
						},
					},
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: awssdk.StringSlice([]string{"sg-id1"}),
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-id1"),
							},
						},
					},
				},
			},
			want: []string{
				"sg-0912f63b",
				"sg-id1",
			},
		},
		{
			name: "describe by id returns error",
			args: args{
				nameOrIDs: []string{
					"sg group name",
					"sg-id",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: awssdk.StringSlice([]string{"sg-id"}),
						},
						err: awserr.New("Describe.Error", "unable to describe security groups", nil),
					},
				},
			},
			wantErr: errors.New("Describe.Error: unable to describe security groups"),
		},
		{
			name: "describe by name returns error",
			args: args{
				nameOrIDs: []string{
					"sg group name",
					"sg-id",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: awssdk.StringSlice([]string{
										"sg group name",
									}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{defaultVPCID}),
								},
							},
						},
						err: awserr.New("Describe.Error", "unable to describe security groups", nil),
					},
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: awssdk.StringSlice([]string{"sg-id"}),
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-id"),
							},
						},
					},
				},
			},
			wantErr: errors.New("Describe.Error: unable to describe security groups"),
		},
		{
			name: "unable to resolve all security groups",
			args: args{
				nameOrIDs: []string{
					"sg group one",
					"sg-id1",
					"sg-id404",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: awssdk.StringSlice([]string{
										"sg group one",
									}),
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{defaultVPCID}),
								},
							},
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-0912f63b"),
							},
						},
					},
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: awssdk.StringSlice([]string{"sg-id1", "sg-id404"}),
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-id1"),
							},
						},
					},
				},
			},
			wantErr: errors.New("couldn't find all securityGroups, nameOrIDs: [sg group one sg-id1 sg-id404], found: [sg-id1 sg-0912f63b]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.args.describeSGCalls {
				ec2Client.EXPECT().DescribeSecurityGroupsAsList(context.Background(), call.req).Return(call.resp, call.err)
			}
			r := &defaultSecurityGroupResolver{
				ec2Client: ec2Client,
				vpcID:     defaultVPCID,
			}
			got, err := r.ResolveViaNameOrID(context.Background(), tt.args.nameOrIDs)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.want, got)
			}
		})
	}
}
