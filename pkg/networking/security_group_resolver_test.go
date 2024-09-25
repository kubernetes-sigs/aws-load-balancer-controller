package networking

import (
	"context"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

func Test_defaultSecurityGroupResolver_ResolveViaNameOrID(t *testing.T) {
	type describeSecurityGroupsAsListCall struct {
		req  *ec2sdk.DescribeSecurityGroupsInput
		resp []ec2types.SecurityGroup
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
							GroupIds: []string{"sg-xx1", "sg-xx2"},
						},
						resp: []ec2types.SecurityGroup{
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
							Filters: []ec2types.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: []string{
										"sg group one",
										"sg group two",
									},
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{defaultVPCID},
								},
							},
						},
						resp: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-0912f63b"),
								Tags: []ec2types.Tag{
									{Key: awssdk.String("Name"), Value: awssdk.String("sg group one")},
								},
							},
							{
								GroupId: awssdk.String("sg-08982de7"),
								Tags: []ec2types.Tag{
									{Key: awssdk.String("Name"), Value: awssdk.String("sg group two")},
								},
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
			name: "single name multiple ids",
			args: args{
				nameOrIDs: []string{
					"sg group one",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []ec2types.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: []string{
										"sg group one",
									},
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{defaultVPCID},
								},
							},
						},
						resp: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-id1"),
								Tags: []ec2types.Tag{
									{Key: awssdk.String("Name"), Value: awssdk.String("sg group one")},
								},
							},
							{
								GroupId: awssdk.String("sg-id2"),
								Tags: []ec2types.Tag{
									{Key: awssdk.String("Name"), Value: awssdk.String("sg group one")},
								},
							},
						},
					},
				},
			},
			want: []string{
				"sg-id1",
				"sg-id2",
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
							Filters: []ec2types.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: []string{
										"sg group one",
									},
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{defaultVPCID},
								},
							},
						},
						resp: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-0912f63b"),
								Tags: []ec2types.Tag{
									{Key: awssdk.String("Name"), Value: awssdk.String("sg group one")},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: []string{"sg-id1"},
						},
						resp: []ec2types.SecurityGroup{
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
					"sg-id",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: []string{"sg-id"},
						},
						err: &smithy.GenericAPIError{Code: "Describe.Error", Message: "unable to describe security groups"},
					},
				},
			},
			wantErr: errors.New("couldn't find all security groups: api error Describe.Error: unable to describe security groups"),
		},
		{
			name: "describe by name returns error",
			args: args{
				nameOrIDs: []string{
					"sg group name",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("tag:Name"),
									Values: []string{"sg group name"},
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{defaultVPCID},
								},
							},
						},
						err: &smithy.GenericAPIError{Code: "Describe.Error", Message: "unable to describe security groups"},
					},
				},
			},
			wantErr: errors.New("couldn't find all security groups: api error Describe.Error: unable to describe security groups"),
		},
		{
			name: "unable to resolve security groups by id",
			args: args{
				nameOrIDs: []string{
					"sg-id1",
					"sg-id404",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: []string{"sg-id1", "sg-id404"},
						},
						resp: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-id1"),
							},
						},
					},
				},
			},
			wantErr: errors.New("couldn't find all security groups: requested ids [sg-id1, sg-id404] but found [sg-id1]"),
		},
		{
			name: "unable to resolve security groups by name",
			args: args{
				nameOrIDs: []string{
					"sg group one",
					"sg group two",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []ec2types.Filter{
								{
									Name: awssdk.String("tag:Name"),
									Values: []string{
										"sg group one",
										"sg group two",
									},
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{defaultVPCID},
								},
							},
						},
						resp: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-0912f63b"),
								Tags: []ec2types.Tag{
									{Key: awssdk.String("Name"), Value: awssdk.String("sg group one")},
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("couldn't find all security groups: requested names [sg group one, sg group two] but found [sg group one]"),
		},
		{
			name: "unable to resolve all security groups by ids and names",
			args: args{
				nameOrIDs: []string{
					"sg-08982de7",
					"sg group one",
				},
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							GroupIds: []string{"sg-08982de7"},
						},
						resp: []ec2types.SecurityGroup{},
					},
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []ec2types.Filter{
								{
									Name:   awssdk.String("tag:Name"),
									Values: []string{"sg group one"},
								},
								{
									Name:   awssdk.String("vpc-id"),
									Values: []string{defaultVPCID},
								},
							},
						},
						resp: []ec2types.SecurityGroup{},
					},
				},
			},
			wantErr: errors.New("couldn't find all security groups: requested ids [sg-08982de7] but found [], requested names [sg group one] but found []"),
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
