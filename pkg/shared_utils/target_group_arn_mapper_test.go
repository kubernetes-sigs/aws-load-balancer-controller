package shared_utils

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"testing"
	"time"
)

func Test_TargetGroupARNMapper(t *testing.T) {
	type describeTargetGroupCall struct {
		req  *elbv2sdk.DescribeTargetGroupsInput
		resp []elbv2types.TargetGroup
		err  error
	}

	type cachedItem struct {
		name string
		arn  string
	}

	testCases := []struct {
		name        string
		tgName      string
		calls       []describeTargetGroupCall
		cachedItems []cachedItem
		expected    string
		expectErr   bool
	}{
		{
			name:   "cold cache",
			tgName: "foo",
			calls: []describeTargetGroupCall{
				{
					req: &elbv2sdk.DescribeTargetGroupsInput{
						Names: []string{"foo"},
					},
					resp: []elbv2types.TargetGroup{
						{
							TargetGroupArn: aws.String("my-arn"),
						},
					},
				},
			},
			expected: "my-arn",
		},
		{
			name:   "warm cache",
			tgName: "foo",
			calls:  []describeTargetGroupCall{},
			cachedItems: []cachedItem{
				{
					name: "foo",
					arn:  "my-arn",
				},
			},
			expected: "my-arn",
		},
		{
			name:   "warm cache but wrong name",
			tgName: "foo",
			calls: []describeTargetGroupCall{
				{
					req: &elbv2sdk.DescribeTargetGroupsInput{
						Names: []string{"foo"},
					},
					resp: []elbv2types.TargetGroup{
						{
							TargetGroupArn: aws.String("my-arn"),
						},
					},
				},
			},
			cachedItems: []cachedItem{
				{
					name: "baz",
					arn:  "other-warn",
				},
			},
			expected: "my-arn",
		},
		{
			name:   "error",
			tgName: "foo",
			calls: []describeTargetGroupCall{
				{
					req: &elbv2sdk.DescribeTargetGroupsInput{
						Names: []string{"foo"},
					},
					resp: []elbv2types.TargetGroup{
						{
							TargetGroupArn: aws.String("my-arn"),
						},
					},
					err: errors.New("error"),
				},
			},
			cachedItems: []cachedItem{
				{
					name: "baz",
					arn:  "other-warn",
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			mapper := NewTargetGroupNameToArnMapper(elbv2Client)

			for _, call := range tc.calls {
				elbv2Client.EXPECT().DescribeTargetGroupsAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			for _, ci := range tc.cachedItems {
				mapper.GetCache().Set(ci.name, ci.arn, time.Minute)
			}

			result, err := mapper.GetArnByName(context.Background(), tc.tgName)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}
