package sg

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
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

type GetSecurityGroupByNameCall struct {
	GroupName string
	Instance  *ec2.SecurityGroup
	Err       error
}

func Test_LBAttachmentDelete(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Instance elbv2.LoadBalancer

		SetSecurityGroupsWithContextCall *SetSecurityGroupsWithContextCall
		GetSecurityGroupByNameCall       *GetSecurityGroupByNameCall
		ExpectedError                    error
	}{
		{
			Name: "delete succeed without modify anything",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("sg-default")},
			},
			GetSecurityGroupByNameCall: &GetSecurityGroupByNameCall{
				GroupName: "default",
				Instance: &ec2.SecurityGroup{
					GroupId: aws.String("sg-default"),
				},
			},
		},
		{
			Name: "delete succeed by modify SG to default one",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("sg-abcd")},
			},
			GetSecurityGroupByNameCall: &GetSecurityGroupByNameCall{
				GroupName: "default",
				Instance: &ec2.SecurityGroup{
					GroupId: aws.String("sg-default"),
				},
			},
			SetSecurityGroupsWithContextCall: &SetSecurityGroupsWithContextCall{
				Input: &elbv2.SetSecurityGroupsInput{
					LoadBalancerArn: aws.String("arn"),
					SecurityGroups:  aws.StringSlice([]string{"sg-default"}),
				},
			},
		},
		{
			Name: "delete failed when get default securityGroup",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("sg-abcd")},
			},
			GetSecurityGroupByNameCall: &GetSecurityGroupByNameCall{
				GroupName: "default",
				Err:       errors.New("GetSecurityGroupByNameCall"),
			},
			ExpectedError: errors.New("failed to get default securityGroup for current vpc due to GetSecurityGroupByNameCall"),
		},
		{
			Name: "delete failed when no default securityGroup",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("sg-abcd")},
			},
			GetSecurityGroupByNameCall: &GetSecurityGroupByNameCall{
				GroupName: "default",
				Instance:  nil,
			},
			ExpectedError: errors.New("failed to get default securityGroup for current vpc due to default security group not found"),
		},
		{
			Name: "delete failed when modify SG to default one",
			Instance: elbv2.LoadBalancer{
				LoadBalancerArn: aws.String("arn"),
				SecurityGroups:  []*string{aws.String("sg-abcd")},
			},
			GetSecurityGroupByNameCall: &GetSecurityGroupByNameCall{
				GroupName: "default",
				Instance: &ec2.SecurityGroup{
					GroupId: aws.String("sg-default"),
				},
			},
			SetSecurityGroupsWithContextCall: &SetSecurityGroupsWithContextCall{
				Input: &elbv2.SetSecurityGroupsInput{
					LoadBalancerArn: aws.String("arn"),
					SecurityGroups:  aws.StringSlice([]string{"sg-default"}),
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
			if tc.GetSecurityGroupByNameCall != nil {
				cloud.On("GetSecurityGroupByName", tc.GetSecurityGroupByNameCall.GroupName).Return(tc.GetSecurityGroupByNameCall.Instance, tc.GetSecurityGroupByNameCall.Err)
			}

			controller := lbAttachmentController{
				cloud: cloud,
			}

			err := controller.Delete(ctx, &tc.Instance)
			assert.Equal(t, err, tc.ExpectedError)
			cloud.AssertExpectations(t)
		})
	}
}
