package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultTaggingManager_ReconcileTags(t *testing.T) {
	type describeTagsWithContextCall struct {
		req  *elbv2sdk.DescribeTagsInput
		resp *elbv2sdk.DescribeTagsOutput
		err  error
	}
	type addTagsWithContextCall struct {
		req  *elbv2sdk.AddTagsInput
		resp *elbv2sdk.AddTagsOutput
		err  error
	}
	type removeTagsWithContextCall struct {
		req  *elbv2sdk.RemoveTagsInput
		resp *elbv2sdk.RemoveTagsOutput
		err  error
	}

	type fields struct {
		describeTagsWithContextCalls []describeTagsWithContextCall
		addTagsWithContextCalls      []addTagsWithContextCall
		removeTagsWithContextCalls   []removeTagsWithContextCall
	}
	type args struct {
		arn         string
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
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("my-arn"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB"),
										},
										{
											Key:   awssdk.String("keyC"),
											Value: awssdk.String("valueC"),
										},
									},
								},
							},
						},
					},
				},
				addTagsWithContextCalls: []addTagsWithContextCall{
					{
						req: &elbv2sdk.AddTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
							Tags: []*elbv2sdk.Tag{
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
				removeTagsWithContextCalls: []removeTagsWithContextCall{
					{
						req: &elbv2sdk.RemoveTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
							TagKeys:      []*string{awssdk.String("keyC")},
						},
					},
				},
			},
			args: args{
				arn: "my-arn",
				desiredTags: map[string]string{
					"keyA": "valueA",
					"keyB": "valueB2",
					"keyD": "valueD",
				},
				opts: nil,
			},
		},
		{
			name: "standard case - with currentTags provided",
			fields: fields{
				describeTagsWithContextCalls: nil,
				addTagsWithContextCalls: []addTagsWithContextCall{
					{
						req: &elbv2sdk.AddTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
							Tags: []*elbv2sdk.Tag{
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
				removeTagsWithContextCalls: []removeTagsWithContextCall{
					{
						req: &elbv2sdk.RemoveTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
							TagKeys:      []*string{awssdk.String("keyC")},
						},
					},
				},
			},
			args: args{
				arn: "my-arn",
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
		},
		{
			name: "ignore specific tag updates and deletes",
			fields: fields{
				describeTagsWithContextCalls: nil,
				addTagsWithContextCalls: []addTagsWithContextCall{
					{
						req: &elbv2sdk.AddTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
							Tags: []*elbv2sdk.Tag{
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
				removeTagsWithContextCalls: []removeTagsWithContextCall{
					{
						req: &elbv2sdk.RemoveTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
							TagKeys:      []*string{awssdk.String("keyF")},
						},
					},
				},
			},
			args: args{
				arn: "my-arn",
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTagsWithContextCalls {
				elbv2Client.EXPECT().DescribeTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.addTagsWithContextCalls {
				elbv2Client.EXPECT().AddTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.removeTagsWithContextCalls {
				elbv2Client.EXPECT().RemoveTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultTaggingManager{
				elbv2Client:           elbv2Client,
				vpcID:                 "vpc-xxxxxxx",
				logger:                &log.NullLogger{},
				describeTagsChunkSize: defaultDescribeTagsChunkSize,
			}
			err := m.ReconcileTags(context.Background(), tt.args.arn, tt.args.desiredTags, tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_defaultTaggingManager_ListLoadBalancers(t *testing.T) {
	type describeLoadBalancersAsListCall struct {
		req  *elbv2sdk.DescribeLoadBalancersInput
		resp []*elbv2sdk.LoadBalancer
		err  error
	}
	type describeTagsWithContextCall struct {
		req  *elbv2sdk.DescribeTagsInput
		resp *elbv2sdk.DescribeTagsOutput
		err  error
	}
	type fields struct {
		describeLoadBalancersAsListCalls []describeLoadBalancersAsListCall
		describeTagsWithContextCalls     []describeTagsWithContextCall
	}
	type args struct {
		tagFilters []tracking.TagFilter
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []LoadBalancerWithTags
		wantErr error
	}{
		{
			name: "2/3 loadBalancers matches single tagFilter; 0 loadBalancers filtered out on VPC ID",
			fields: fields{
				describeLoadBalancersAsListCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []*elbv2sdk.LoadBalancer{
							{
								LoadBalancerArn: awssdk.String("lb-1"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-2"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-3"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"lb-1", "lb-2", "lb-3"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("lb-1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("lb-2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("lb-3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA1", "valueA3"},
					},
				},
			},
			want: []LoadBalancerWithTags{
				{
					LoadBalancer: &elbv2sdk.LoadBalancer{LoadBalancerArn: awssdk.String("lb-1"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA1",
						"keyB": "valueB1",
					},
				},
				{
					LoadBalancer: &elbv2sdk.LoadBalancer{LoadBalancerArn: awssdk.String("lb-3"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA3",
						"keyB": "valueB3",
					},
				},
			},
		},
		{
			name: "1/3 loadBalancers matches single tagFilter; 1 loadBalancer filtered out on VPC ID",
			fields: fields{
				describeLoadBalancersAsListCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []*elbv2sdk.LoadBalancer{
							{
								LoadBalancerArn: awssdk.String("lb-1"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-2"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-3"),
								VpcId:           awssdk.String("vpc-aaaaaaa"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"lb-1", "lb-2"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("lb-1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("lb-2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA1", "valueA3"},
					},
				},
			},
			want: []LoadBalancerWithTags{
				{
					LoadBalancer: &elbv2sdk.LoadBalancer{LoadBalancerArn: awssdk.String("lb-1"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA1",
						"keyB": "valueB1",
					},
				},
			},
		},
		{
			name: "1/3 loadBalancers matches single tagFilter; 2 loadBalancers filtered out on VPC ID",
			fields: fields{
				describeLoadBalancersAsListCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []*elbv2sdk.LoadBalancer{
							{
								LoadBalancerArn: awssdk.String("lb-1"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-2"),
								VpcId:           awssdk.String("vpc-yyyyyyy"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-3"),
								VpcId:           awssdk.String("vpc-aaaaaaa"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"lb-1"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("lb-1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA1", "valueA3"},
					},
				},
			},
			want: []LoadBalancerWithTags{
				{
					LoadBalancer: &elbv2sdk.LoadBalancer{LoadBalancerArn: awssdk.String("lb-1"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA1",
						"keyB": "valueB1",
					},
				},
			},
		},
		{
			name: "0/3 loadBalancers matches single tagFilter; 0 loadBalancers filtered out on VPC ID",
			fields: fields{
				describeLoadBalancersAsListCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{},
						resp: []*elbv2sdk.LoadBalancer{
							{
								LoadBalancerArn: awssdk.String("lb-1"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-2"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
							{
								LoadBalancerArn: awssdk.String("lb-3"),
								VpcId:           awssdk.String("vpc-xxxxxxx"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"lb-1", "lb-2", "lb-3"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("lb-1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("lb-2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("lb-3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA4"},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeLoadBalancersAsListCalls {
				elbv2Client.EXPECT().DescribeLoadBalancersAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.describeTagsWithContextCalls {
				elbv2Client.EXPECT().DescribeTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultTaggingManager{
				elbv2Client:           elbv2Client,
				vpcID:                 "vpc-xxxxxxx",
				describeTagsChunkSize: defaultDescribeTagsChunkSize,
			}
			got, err := m.ListLoadBalancers(context.Background(), tt.args.tagFilters...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultTaggingManager_ListTargetGroups(t *testing.T) {
	type describeTargetGroupsAsListCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []*elbv2sdk.TargetGroup
		err  error
	}
	type describeTagsWithContextCall struct {
		req  *elbv2sdk.DescribeTagsInput
		resp *elbv2sdk.DescribeTagsOutput
		err  error
	}
	type fields struct {
		describeTargetGroupsAsListCalls []describeTargetGroupsAsListCall
		describeTagsWithContextCalls    []describeTagsWithContextCall
	}
	type args struct {
		tagFilters []tracking.TagFilter
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []TargetGroupWithTags
		wantErr error
	}{
		{
			name: "2/3 targetGroups matches single tagFilter; 0 targetGroups filtered out on VPC ID",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{},
						resp: []*elbv2sdk.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-2"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-3"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"tg-1", "tg-2", "tg-3"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("tg-1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA1", "valueA3"},
					},
				},
			},
			want: []TargetGroupWithTags{
				{
					TargetGroup: &elbv2sdk.TargetGroup{TargetGroupArn: awssdk.String("tg-1"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA1",
						"keyB": "valueB1",
					},
				},
				{
					TargetGroup: &elbv2sdk.TargetGroup{TargetGroupArn: awssdk.String("tg-3"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA3",
						"keyB": "valueB3",
					},
				},
			},
		},
		{
			name: "1/3 targetGroups matches single tagFilter; 1 targetGroup filtered out on VPC ID",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{},
						resp: []*elbv2sdk.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								VpcId:          awssdk.String("vpc-yyyyyyy"),
							},
							{
								TargetGroupArn: awssdk.String("tg-2"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-3"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"tg-2", "tg-3"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("tg-2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA1", "valueA3"},
					},
				},
			},
			want: []TargetGroupWithTags{
				{
					TargetGroup: &elbv2sdk.TargetGroup{TargetGroupArn: awssdk.String("tg-3"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA3",
						"keyB": "valueB3",
					},
				},
			},
		},
		{
			name: "1/3 targetGroups matches single tagFilter; 2 targetGroups filtered out on VPC ID",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{},
						resp: []*elbv2sdk.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								VpcId:          awssdk.String("vpc-yyyyyyy"),
							},
							{
								TargetGroupArn: awssdk.String("tg-2"),
								VpcId:          awssdk.String("vpc-ccccccc"),
							},
							{
								TargetGroupArn: awssdk.String("tg-3"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"tg-3"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("tg-3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA1", "valueA3"},
					},
				},
			},
			want: []TargetGroupWithTags{
				{
					TargetGroup: &elbv2sdk.TargetGroup{TargetGroupArn: awssdk.String("tg-3"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA3",
						"keyB": "valueB3",
					},
				},
			},
		},
		{
			name: "0/3 targetGroups matches single tagFilter; 0 targetGroups filtered out on VPC ID",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{},
						resp: []*elbv2sdk.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-2"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-3"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"tg-1", "tg-2", "tg-3"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("tg-1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA4"},
					},
				},
			},
			want: nil,
		},
		{
			name: "2/4 targetGroups matches first tagFilter, 2/4 targetGroups matches second tagFilter",
			fields: fields{
				describeTargetGroupsAsListCalls: []describeTargetGroupsAsListCall{
					{
						req: &elbv2sdk.DescribeTargetGroupsInput{},
						resp: []*elbv2sdk.TargetGroup{
							{
								TargetGroupArn: awssdk.String("tg-1"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-2"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-3"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
							{
								TargetGroupArn: awssdk.String("tg-4"),
								VpcId:          awssdk.String("vpc-xxxxxxx"),
							},
						},
					},
				},
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: awssdk.StringSlice([]string{"tg-1", "tg-2", "tg-3", "tg-4"}),
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("tg-1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("tg-4"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA4"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB4"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tagFilters: []tracking.TagFilter{
					{
						"keyA": {"valueA1", "valueA2"},
					},
					{
						"keyA": {"valueA2", "valueA4"},
					},
				},
			},
			want: []TargetGroupWithTags{
				{
					TargetGroup: &elbv2sdk.TargetGroup{TargetGroupArn: awssdk.String("tg-1"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA1",
						"keyB": "valueB1",
					},
				},
				{
					TargetGroup: &elbv2sdk.TargetGroup{TargetGroupArn: awssdk.String("tg-2"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA2",
						"keyB": "valueB2",
					},
				},
				{
					TargetGroup: &elbv2sdk.TargetGroup{TargetGroupArn: awssdk.String("tg-4"), VpcId: awssdk.String("vpc-xxxxxxx")},
					Tags: map[string]string{
						"keyA": "valueA4",
						"keyB": "valueB4",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetGroupsAsListCalls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.describeTagsWithContextCalls {
				elbv2Client.EXPECT().DescribeTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultTaggingManager{
				elbv2Client:           elbv2Client,
				vpcID:                 "vpc-xxxxxxx",
				describeTagsChunkSize: defaultDescribeTagsChunkSize,
			}
			got, err := m.ListTargetGroups(context.Background(), tt.args.tagFilters...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultTaggingManager_describeResourceTags(t *testing.T) {
	type describeTagsWithContextCall struct {
		req  *elbv2sdk.DescribeTagsInput
		resp *elbv2sdk.DescribeTagsOutput
		err  error
	}
	type fields struct {
		describeTagsWithContextCalls []describeTagsWithContextCall
	}
	type args struct {
		arns []string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]map[string]string
		wantErr error
	}{
		{
			name: "single resource",
			fields: fields{
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn")},
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("my-arn"),
									Tags: []*elbv2sdk.Tag{
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
							},
						},
					},
				},
			},
			args: args{
				arns: []string{"my-arn"},
			},
			want: map[string]map[string]string{
				"my-arn": {
					"keyA": "valueA",
					"keyB": "valueB",
				},
			},
		},
		{
			name: "multiple resource(more than chunkSize)",
			fields: fields{
				describeTagsWithContextCalls: []describeTagsWithContextCall{
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn1"), awssdk.String("my-arn2")},
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("my-arn1"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA1"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB1"),
										},
									},
								},
								{
									ResourceArn: awssdk.String("my-arn2"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA2"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB2"),
										},
									},
								},
							},
						},
					},
					{
						req: &elbv2sdk.DescribeTagsInput{
							ResourceArns: []*string{awssdk.String("my-arn3")},
						},
						resp: &elbv2sdk.DescribeTagsOutput{
							TagDescriptions: []*elbv2sdk.TagDescription{
								{
									ResourceArn: awssdk.String("my-arn3"),
									Tags: []*elbv2sdk.Tag{
										{
											Key:   awssdk.String("keyA"),
											Value: awssdk.String("valueA3"),
										},
										{
											Key:   awssdk.String("keyB"),
											Value: awssdk.String("valueB3"),
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				arns: []string{"my-arn1", "my-arn2", "my-arn3"},
			},
			want: map[string]map[string]string{
				"my-arn1": {
					"keyA": "valueA1",
					"keyB": "valueB1",
				},
				"my-arn2": {
					"keyA": "valueA2",
					"keyB": "valueB2",
				},
				"my-arn3": {
					"keyA": "valueA3",
					"keyB": "valueB3",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTagsWithContextCalls {
				elbv2Client.EXPECT().DescribeTagsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultTaggingManager{
				elbv2Client:           elbv2Client,
				vpcID:                 "vpc-xxxxxxx",
				describeTagsChunkSize: 2,
			}
			got, err := m.describeResourceTags(context.Background(), tt.args.arns)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
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
		want []*elbv2sdk.Tag
	}{
		{
			name: "non-empty case",
			args: args{
				tags: map[string]string{
					"keyA": "valueA",
					"keyB": "valueB",
				},
			},
			want: []*elbv2sdk.Tag{
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
			name: "nil case",
			args: args{tags: nil},
			want: nil,
		},
		{
			name: "empty case",
			args: args{tags: map[string]string{}},
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

func Test_convertSDKTagsToTags(t *testing.T) {
	type args struct {
		sdkTags []*elbv2sdk.Tag
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "non-empty case",
			args: args{
				sdkTags: []*elbv2sdk.Tag{
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
			want: map[string]string{
				"keyA": "valueA",
				"keyB": "valueB",
			},
		},
		{
			name: "nil case",
			args: args{
				sdkTags: nil,
			},
			want: map[string]string{},
		},
		{
			name: "empty case",
			args: args{
				sdkTags: []*elbv2sdk.Tag{},
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSDKTagsToTags(tt.args.sdkTags)
			assert.Equal(t, tt.want, got)
		})
	}
}
