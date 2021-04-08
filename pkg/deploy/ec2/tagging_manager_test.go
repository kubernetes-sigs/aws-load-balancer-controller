package ec2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultTaggingManager_ReconcileTags(t *testing.T) {
	type createTagsWithContextCall struct {
		req  *ec2sdk.CreateTagsInput
		resp *ec2sdk.CreateTagsOutput
		err  error
	}
	type deleteTagsWithContextCall struct {
		req  *ec2sdk.DeleteTagsInput
		resp *ec2sdk.DeleteTagsOutput
		err  error
	}

	type fields struct {
		createTagsWithContextCalls []createTagsWithContextCall
		deleteTagsWithContextCalls []deleteTagsWithContextCall
	}
	type args struct {
		resID       string
		desiredTags map[string]string
		opts        []ReconcileTagsOption
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "standard case - add/update/remove tags",
			fields: fields{
				createTagsWithContextCalls: []createTagsWithContextCall{
					{
						req: &ec2sdk.CreateTagsInput{
							Resources: awssdk.StringSlice([]string{"sg-a"}),
							Tags: []*ec2sdk.Tag{
								{
									Key:   awssdk.String("keyB"),
									Value: awssdk.String("valueB2"),
								},
								{
									Key:   awssdk.String("keyD"),
									Value: awssdk.String("valueD"),
								},
							},
						},
					},
				},
				deleteTagsWithContextCalls: []deleteTagsWithContextCall{
					{
						req: &ec2sdk.DeleteTagsInput{
							Resources: awssdk.StringSlice([]string{"sg-a"}),
							Tags: []*ec2sdk.Tag{
								{
									Key:   awssdk.String("keyC"),
									Value: awssdk.String("valueC"),
								},
							},
						},
					},
				},
			},
			args: args{
				resID: "sg-a",
				desiredTags: map[string]string{
					"keyA": "valueA",
					"keyB": "valueB2",
					"keyD": "valueD",
				},
				opts: []ReconcileTagsOption{
					WithCurrentTags(map[string]string{
						"keyA": "valueA",
						"keyB": "valueB",
						"keyC": "valueC",
					}),
				},
			},
			wantErr: nil,
		},
		{
			name: "ignore specific tag updates and deletes",
			fields: fields{
				createTagsWithContextCalls: []createTagsWithContextCall{
					{
						req: &ec2sdk.CreateTagsInput{
							Resources: awssdk.StringSlice([]string{"sg-a"}),
							Tags: []*ec2sdk.Tag{
								{
									Key:   awssdk.String("keyC"),
									Value: awssdk.String("valueC2"),
								},
								{
									Key:   awssdk.String("keyD"),
									Value: awssdk.String("valueD"),
								},
							},
						},
					},
				},
				deleteTagsWithContextCalls: []deleteTagsWithContextCall{
					{
						req: &ec2sdk.DeleteTagsInput{
							Resources: awssdk.StringSlice([]string{"sg-a"}),
							Tags: []*ec2sdk.Tag{
								{
									Key:   awssdk.String("keyF"),
									Value: awssdk.String("valueF"),
								},
							},
						},
					},
				},
			},
			args: args{
				resID: "sg-a",
				desiredTags: map[string]string{
					"keyA": "valueA",
					"keyB": "valueB2",
					"keyC": "valueC2",
					"keyD": "valueD",
				},
				opts: []ReconcileTagsOption{
					WithCurrentTags(map[string]string{
						"keyA": "valueA",
						"keyB": "valueB",
						"keyC": "valueC",
						"keyE": "valueE",
						"keyF": "valueF",
					}),
					WithIgnoredTagKeys([]string{"keyB", "keyE"}),
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.createTagsWithContextCalls {
				ec2Client.EXPECT().CreateTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.deleteTagsWithContextCalls {
				ec2Client.EXPECT().DeleteTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultTaggingManager{
				ec2Client: ec2Client,
				logger:    &log.NullLogger{},
			}
			err := m.ReconcileTags(context.Background(), tt.args.resID, tt.args.desiredTags, tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}

func Test_defaultTaggingManager_ListSecurityGroups(t *testing.T) {
	type fetchSGInfosByRequestCall struct {
		req  *ec2sdk.DescribeSecurityGroupsInput
		resp map[string]networking.SecurityGroupInfo
		err  error
	}
	type fields struct {
		fetchSGInfosByRequestCalls []fetchSGInfosByRequestCall
	}
	type args struct {
		tagFilters []tracking.TagFilter
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []networking.SecurityGroupInfo
		wantErr error
	}{
		{
			name: "with a single tagFilter",
			fields: fields{
				fetchSGInfosByRequestCalls: []fetchSGInfosByRequestCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-xxxxxxx"}),
								},
								{
									Name:   awssdk.String("tag:keyA"),
									Values: awssdk.StringSlice([]string{"valueA"}),
								},
								{
									Name:   awssdk.String("tag:keyB"),
									Values: awssdk.StringSlice([]string{"valueB1", "valueB2"}),
								},
								{
									Name:   awssdk.String("tag-key"),
									Values: awssdk.StringSlice([]string{"keyC"}),
								},
							},
						},
						resp: map[string]networking.SecurityGroupInfo{
							"sg-a": {
								SecurityGroupID: "sg-a",
								Tags: map[string]string{
									"keyA": "valueA",
									"keyB": "valueB1",
									"keyC": "valueC",
									"keyD": "valueD",
								},
							},
							"sg-b": {
								SecurityGroupID: "sg-b",
								Tags: map[string]string{
									"keyA": "valueA",
									"keyB": "valueB2",
									"keyC": "valueC",
									"keyD": "valueD",
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": []string{"valueA"},
						"keyB": []string{"valueB1", "valueB2"},
						"keyC": nil,
					},
				},
			},
			want: []networking.SecurityGroupInfo{
				{
					SecurityGroupID: "sg-a",
					Tags: map[string]string{
						"keyA": "valueA",
						"keyB": "valueB1",
						"keyC": "valueC",
						"keyD": "valueD",
					},
				},
				{
					SecurityGroupID: "sg-b",
					Tags: map[string]string{
						"keyA": "valueA",
						"keyB": "valueB2",
						"keyC": "valueC",
						"keyD": "valueD",
					},
				},
			},
		},
		{
			name: "with two tagFilter",
			fields: fields{
				fetchSGInfosByRequestCalls: []fetchSGInfosByRequestCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-xxxxxxx"}),
								},
								{
									Name:   awssdk.String("tag:keyA"),
									Values: awssdk.StringSlice([]string{"valueA"}),
								},
								{
									Name:   awssdk.String("tag:keyB"),
									Values: awssdk.StringSlice([]string{"valueB1", "valueB2"}),
								},
								{
									Name:   awssdk.String("tag-key"),
									Values: awssdk.StringSlice([]string{"keyC"}),
								},
							},
						},
						resp: map[string]networking.SecurityGroupInfo{
							"sg-a": {
								SecurityGroupID: "sg-a",
								Tags: map[string]string{
									"keyA": "valueA",
									"keyB": "valueB1",
									"keyC": "valueC",
									"keyD": "valueD",
								},
							},
							"sg-b": {
								SecurityGroupID: "sg-b",
								Tags: map[string]string{
									"keyA": "valueA",
									"keyB": "valueB2",
									"keyC": "valueC",
									"keyD": "valueD",
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-xxxxxxx"}),
								},
								{
									Name:   awssdk.String("tag:keyA"),
									Values: awssdk.StringSlice([]string{"valueA"}),
								},
								{
									Name:   awssdk.String("tag:keyB"),
									Values: awssdk.StringSlice([]string{"valueB2", "valueB3"}),
								},
								{
									Name:   awssdk.String("tag-key"),
									Values: awssdk.StringSlice([]string{"keyC"}),
								},
							},
						},
						resp: map[string]networking.SecurityGroupInfo{
							"sg-b": {
								SecurityGroupID: "sg-b",
								Tags: map[string]string{
									"keyA": "valueA",
									"keyB": "valueB2",
									"keyC": "valueC",
									"keyD": "valueD",
								},
							},
							"sg-c": {
								SecurityGroupID: "sg-c",
								Tags: map[string]string{
									"keyA": "valueA",
									"keyB": "valueB3",
									"keyC": "valueC",
									"keyD": "valueD",
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": []string{"valueA"},
						"keyB": []string{"valueB1", "valueB2"},
						"keyC": nil,
					},
					{
						"keyA": []string{"valueA"},
						"keyB": []string{"valueB2", "valueB3"},
						"keyC": nil,
					},
				},
			},
			want: []networking.SecurityGroupInfo{
				{
					SecurityGroupID: "sg-a",
					Tags: map[string]string{
						"keyA": "valueA",
						"keyB": "valueB1",
						"keyC": "valueC",
						"keyD": "valueD",
					},
				},
				{
					SecurityGroupID: "sg-b",
					Tags: map[string]string{
						"keyA": "valueA",
						"keyB": "valueB2",
						"keyC": "valueC",
						"keyD": "valueD",
					},
				},
				{
					SecurityGroupID: "sg-c",
					Tags: map[string]string{
						"keyA": "valueA",
						"keyB": "valueB3",
						"keyC": "valueC",
						"keyD": "valueD",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			networkingSGManager := networking.NewMockSecurityGroupManager(ctrl)
			for _, call := range tt.fields.fetchSGInfosByRequestCalls {
				networkingSGManager.EXPECT().FetchSGInfosByRequest(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			m := &defaultTaggingManager{
				networkingSGManager: networkingSGManager,
				vpcID:               "vpc-xxxxxxx",
			}
			got, err := m.ListSecurityGroups(context.Background(), tt.args.tagFilters...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opts := cmpopts.SortSlices(func(lhs networking.SecurityGroupInfo, rhs networking.SecurityGroupInfo) bool {
					return lhs.SecurityGroupID < rhs.SecurityGroupID
				})
				assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			}
		})
	}
}

func Test_convertTagsToSDKTags(t *testing.T) {
	type args struct {
		tags map[string]string
	}
	tests := []struct {
		name string
		args args
		want []*ec2sdk.Tag
	}{
		{
			name: "non-empty tags",
			args: args{
				tags: map[string]string{
					"keyA": "valueA",
					"keyB": "valueB",
				},
			},
			want: []*ec2sdk.Tag{
				{
					Key:   awssdk.String("keyA"),
					Value: awssdk.String("valueA"),
				},
				{
					Key:   awssdk.String("keyB"),
					Value: awssdk.String("valueB"),
				},
			},
		},
		{
			name: "nil tags",
			args: args{
				tags: nil,
			},
			want: nil,
		},
		{
			name: "empty tags",
			args: args{
				tags: map[string]string{},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertTagsToSDKTags(tt.args.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}
