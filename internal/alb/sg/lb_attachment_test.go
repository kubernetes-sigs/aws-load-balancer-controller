package sg

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/magiconair/properties/assert"
)

type SetSecurityGroupsWithContextCall struct {
	Input *elbv2.SetSecurityGroupsInput
	Err   error
}

func Test_LBAttachmentReconcile(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Instance elbv2.LoadBalancer
		GroupIDs []string

		SetSecurityGroupsWithContextCall *SetSecurityGroupsWithContextCall
		ExpectedError                    error
	}{
		{
			Name: "reconcile succeed without modify anything",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("sg-1")},
			},
			GroupIDs: []string{"sg-1"},
		},
		{
			Name: "reconcile succeed by modify SG",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("default")},
			},
			GroupIDs: []string{"sg-1", "sg-2"},
			SetSecurityGroupsWithContextCall: &SetSecurityGroupsWithContextCall{
				Input: &elbv2.SetSecurityGroupsInput{
					LoadBalancerArn: aws.String("arn"),
					SecurityGroups:  aws.StringSlice([]string{"sg-1", "sg-2"}),
				},
			},
		},
		{
			Name: "reconcile failed when modify SG",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("default")},
			},
			GroupIDs: []string{"sg-1", "sg-2"},
			SetSecurityGroupsWithContextCall: &SetSecurityGroupsWithContextCall{
				Input: &elbv2.SetSecurityGroupsInput{
					LoadBalancerArn: aws.String("arn"),
					SecurityGroups:  aws.StringSlice([]string{"sg-1", "sg-2"}),
				},
				Err: errors.New("SetSecurityGroupsWithContextCall"),
			},
			ExpectedError: errors.New("SetSecurityGroupsWithContextCall"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.SetSecurityGroupsWithContextCall != nil {
				cloud.On("SetSecurityGroupsWithContext", ctx, tc.SetSecurityGroupsWithContextCall.Input).Return(nil, tc.SetSecurityGroupsWithContextCall.Err)
			}

			controller := lbAttachmentController{
				cloud: cloud,
			}

			err := controller.Reconcile(ctx, &tc.Instance, tc.GroupIDs)
			assert.Equal(t, err, tc.ExpectedError)
			cloud.AssertExpectations(t)
		})
	}
}
