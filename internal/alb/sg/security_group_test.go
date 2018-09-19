package sg

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

type GetSecurityGroupByIDCall struct {
	GroupID  *string
	Instance *ec2.SecurityGroup
	Err      error
}

type GetSecurityGroupByNameCall struct {
	GroupName *string
	Instance  *ec2.SecurityGroup
	Err       error
}

type RevokeSecurityGroupIngressCall struct {
	Input *ec2.RevokeSecurityGroupIngressInput
	Err   error
}

type AuthorizeSecurityGroupIngressCall struct {
	Input *ec2.AuthorizeSecurityGroupIngressInput
	Err   error
}

type CreateSecurityGroupCall struct {
	Input  *ec2.CreateSecurityGroupInput
	Output *ec2.CreateSecurityGroupOutput
	Err    error
}

type CreateTagsCall struct {
	Input *ec2.CreateTagsInput
	Err   error
}

type DeleteSecurityGroupByIDCall struct {
	GroupID *string
	Err     error
}

func TestReconcile(t *testing.T) {
	for _, tc := range []struct {
		Name                              string
		SecurityGroup                     SecurityGroup
		GetSecurityGroupByIDCall          GetSecurityGroupByIDCall
		GetSecurityGroupByNameCall        GetSecurityGroupByNameCall
		RevokeSecurityGroupIngressCall    RevokeSecurityGroupIngressCall
		AuthorizeSecurityGroupIngressCall AuthorizeSecurityGroupIngressCall
		CreateSecurityGroupCall           CreateSecurityGroupCall
		CreateTagsCall                    CreateTagsCall
		ExpectedError                     error
	}{
		{
			Name: "securityGroupID doesn't exist",
			SecurityGroup: SecurityGroup{
				GroupID:            aws.String("groupID"),
				GroupName:          nil,
				InboundPermissions: nil,
			},
			GetSecurityGroupByIDCall: GetSecurityGroupByIDCall{
				GroupID:  aws.String("groupID"),
				Instance: nil,
				Err:      nil,
			},
			ExpectedError: errors.New("securityGroup groupID doesn't exist"),
		},
		{
			Name: "securityGroupID and securityGroupName both unspecified",
			SecurityGroup: SecurityGroup{
				GroupID:            nil,
				GroupName:          nil,
				InboundPermissions: nil,
			},
			ExpectedError: errors.New("Either GroupID or GroupName must be specified"),
		},
		{
			Name: "happy case of reconcile by modify existing sg instance by ID",
			SecurityGroup: SecurityGroup{
				GroupID:   aws.String("groupID"),
				GroupName: nil,
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByIDCall: GetSecurityGroupByIDCall{
				GroupID: aws.String("groupID"),
				Instance: &ec2.SecurityGroup{
					GroupId:   aws.String("groupID"),
					GroupName: aws.String("groupName"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupA"),
								},
							},
						},
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			RevokeSecurityGroupIngressCall: RevokeSecurityGroupIngressCall{
				Input: &ec2.RevokeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			AuthorizeSecurityGroupIngressCall: AuthorizeSecurityGroupIngressCall{
				Input: &ec2.AuthorizeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupB"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			ExpectedError: nil,
		},
		{
			Name: "happy case of reconcile by modify existing sg instance by name",
			SecurityGroup: SecurityGroup{
				GroupID:   nil,
				GroupName: aws.String("groupName"),
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByNameCall: GetSecurityGroupByNameCall{
				GroupName: aws.String("groupName"),
				Instance: &ec2.SecurityGroup{
					GroupId:   aws.String("groupID"),
					GroupName: aws.String("groupName"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupA"),
								},
							},
						},
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			RevokeSecurityGroupIngressCall: RevokeSecurityGroupIngressCall{
				Input: &ec2.RevokeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			AuthorizeSecurityGroupIngressCall: AuthorizeSecurityGroupIngressCall{
				Input: &ec2.AuthorizeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupB"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			ExpectedError: nil,
		},
		{
			Name: "reconcile by modify existing sg instance failed when revoke permissions",
			SecurityGroup: SecurityGroup{
				GroupID:   aws.String("groupID"),
				GroupName: nil,
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByIDCall: GetSecurityGroupByIDCall{
				GroupID: aws.String("groupID"),
				Instance: &ec2.SecurityGroup{
					GroupId:   aws.String("groupID"),
					GroupName: aws.String("groupName"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupA"),
								},
							},
						},
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			RevokeSecurityGroupIngressCall: RevokeSecurityGroupIngressCall{
				Input: &ec2.RevokeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: errors.New("i just failed"),
			},
			ExpectedError: errors.New("failed to revoke inbound permissions due to i just failed"),
		},
		{
			Name: "reconcile by modify existing sg instance failed when granting permissions",
			SecurityGroup: SecurityGroup{
				GroupID:   aws.String("groupID"),
				GroupName: nil,
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByIDCall: GetSecurityGroupByIDCall{
				GroupID: aws.String("groupID"),
				Instance: &ec2.SecurityGroup{
					GroupId:   aws.String("groupID"),
					GroupName: aws.String("groupName"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupA"),
								},
							},
						},
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			RevokeSecurityGroupIngressCall: RevokeSecurityGroupIngressCall{
				Input: &ec2.RevokeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			AuthorizeSecurityGroupIngressCall: AuthorizeSecurityGroupIngressCall{
				Input: &ec2.AuthorizeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupB"),
								},
							},
						},
					},
				},
				Err: errors.New("i just failed"),
			},
			ExpectedError: errors.New("failed to grant inbound permissions due to i just failed"),
		},
		{
			Name: "happy case of reconcile by new sg instance",
			SecurityGroup: SecurityGroup{
				GroupID:   nil,
				GroupName: aws.String("groupName"),
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByNameCall: GetSecurityGroupByNameCall{
				GroupName: aws.String("groupName"),
				Instance:  nil,
				Err:       nil,
			},
			AuthorizeSecurityGroupIngressCall: AuthorizeSecurityGroupIngressCall{
				Input: &ec2.AuthorizeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupA"),
								},
							},
						},
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupB"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			CreateSecurityGroupCall: CreateSecurityGroupCall{
				Input: &ec2.CreateSecurityGroupInput{
					VpcId:       aws.String("vpc-id"),
					GroupName:   aws.String("groupName"),
					Description: aws.String("Instance SecurityGroup created by alb-ingress-controller"),
				},
				Output: &ec2.CreateSecurityGroupOutput{
					GroupId: aws.String("groupID"),
				},
				Err: nil,
			},
			CreateTagsCall: CreateTagsCall{
				Input: &ec2.CreateTagsInput{
					Resources: []*string{aws.String("groupID")},
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("groupName"),
						},
						{
							Key:   aws.String(albec2.ManagedByKey),
							Value: aws.String(albec2.ManagedByValue),
						},
					},
				},
				Err: nil,
			},
			ExpectedError: nil,
		},
		{
			Name: "reconcile by new sg instance failed when creating new sg",
			SecurityGroup: SecurityGroup{
				GroupID:   nil,
				GroupName: aws.String("groupName"),
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByNameCall: GetSecurityGroupByNameCall{
				GroupName: aws.String("groupName"),
				Instance:  nil,
				Err:       nil,
			},
			CreateSecurityGroupCall: CreateSecurityGroupCall{
				Input: &ec2.CreateSecurityGroupInput{
					VpcId:       aws.String("vpc-id"),
					GroupName:   aws.String("groupName"),
					Description: aws.String("Instance SecurityGroup created by alb-ingress-controller"),
				},
				Output: nil,
				Err:    errors.New("i just failed"),
			},
			ExpectedError: errors.New("i just failed"),
		},
		{
			Name: "reconcile by new sg instance failed when granting permission",
			SecurityGroup: SecurityGroup{
				GroupID:   nil,
				GroupName: aws.String("groupName"),
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByNameCall: GetSecurityGroupByNameCall{
				GroupName: aws.String("groupName"),
				Instance:  nil,
				Err:       nil,
			},
			AuthorizeSecurityGroupIngressCall: AuthorizeSecurityGroupIngressCall{
				Input: &ec2.AuthorizeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupA"),
								},
							},
						},
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupB"),
								},
							},
						},
					},
				},
				Err: errors.New("i just failed"),
			},
			CreateSecurityGroupCall: CreateSecurityGroupCall{
				Input: &ec2.CreateSecurityGroupInput{
					VpcId:       aws.String("vpc-id"),
					GroupName:   aws.String("groupName"),
					Description: aws.String("Instance SecurityGroup created by alb-ingress-controller"),
				},
				Output: &ec2.CreateSecurityGroupOutput{
					GroupId: aws.String("groupID"),
				},
				Err: nil,
			},
			ExpectedError: errors.New("i just failed"),
		},
		{
			Name: "reconcile by new sg instance failed when creating tags",
			SecurityGroup: SecurityGroup{
				GroupID:   nil,
				GroupName: aws.String("groupName"),
				InboundPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupA"),
							},
						},
					},
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(80),
						ToPort:     aws.Int64(81),
						UserIdGroupPairs: []*ec2.UserIdGroupPair{
							{
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByNameCall: GetSecurityGroupByNameCall{
				GroupName: aws.String("groupName"),
				Instance:  nil,
				Err:       nil,
			},
			AuthorizeSecurityGroupIngressCall: AuthorizeSecurityGroupIngressCall{
				Input: &ec2.AuthorizeSecurityGroupIngressInput{
					GroupId: aws.String("groupID"),
					IpPermissions: []*ec2.IpPermission{
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupA"),
								},
							},
						},
						{
							IpProtocol: aws.String("tcp"),
							FromPort:   aws.Int64(80),
							ToPort:     aws.Int64(81),
							UserIdGroupPairs: []*ec2.UserIdGroupPair{
								{
									GroupId: aws.String("groupB"),
								},
							},
						},
					},
				},
				Err: nil,
			},
			CreateSecurityGroupCall: CreateSecurityGroupCall{
				Input: &ec2.CreateSecurityGroupInput{
					VpcId:       aws.String("vpc-id"),
					GroupName:   aws.String("groupName"),
					Description: aws.String("Instance SecurityGroup created by alb-ingress-controller"),
				},
				Output: &ec2.CreateSecurityGroupOutput{
					GroupId: aws.String("groupID"),
				},
				Err: nil,
			},
			CreateTagsCall: CreateTagsCall{
				Input: &ec2.CreateTagsInput{
					Resources: []*string{aws.String("groupID")},
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("groupName"),
						},
						{
							Key:   aws.String(albec2.ManagedByKey),
							Value: aws.String(albec2.ManagedByValue),
						},
					},
				},
				Err: errors.New("i just failed"),
			},
			ExpectedError: errors.New("i just failed"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ec2 := &mocks.EC2API{}
			if tc.GetSecurityGroupByIDCall.GroupID != nil {
				ec2.On("GetSecurityGroupByID", *tc.GetSecurityGroupByIDCall.GroupID).Return(tc.GetSecurityGroupByIDCall.Instance, tc.GetSecurityGroupByIDCall.Err)
			}
			if tc.GetSecurityGroupByNameCall.GroupName != nil {
				ec2.On("GetVPCID").Return(aws.String("vpc-id"), nil)
				ec2.On("GetSecurityGroupByName", "vpc-id", *tc.GetSecurityGroupByNameCall.GroupName).Return(tc.GetSecurityGroupByNameCall.Instance, tc.GetSecurityGroupByNameCall.Err)
			}
			if tc.RevokeSecurityGroupIngressCall.Input != nil {
				ec2.On("RevokeSecurityGroupIngress", tc.RevokeSecurityGroupIngressCall.Input).Return(nil, tc.RevokeSecurityGroupIngressCall.Err)
			}
			if tc.AuthorizeSecurityGroupIngressCall.Input != nil {
				ec2.On("AuthorizeSecurityGroupIngress", tc.AuthorizeSecurityGroupIngressCall.Input).Return(nil, tc.AuthorizeSecurityGroupIngressCall.Err)
			}
			if tc.CreateSecurityGroupCall.Input != nil {
				ec2.On("CreateSecurityGroup", tc.CreateSecurityGroupCall.Input).Return(tc.CreateSecurityGroupCall.Output, tc.CreateSecurityGroupCall.Err)
			}
			if tc.CreateTagsCall.Input != nil {
				ec2.On("CreateTags", tc.CreateTagsCall.Input).Return(nil, tc.CreateTagsCall.Err)
			}

			controller := &securityGroupController{
				ec2:    ec2,
				logger: log.New("test"),
			}
			err := controller.Reconcile(&tc.SecurityGroup)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "groupID", aws.StringValue(tc.SecurityGroup.GroupID))
				assert.Equal(t, "groupName", aws.StringValue(tc.SecurityGroup.GroupName))
			}
			ec2.AssertExpectations(t)
		})
	}
}

func TestDelete(t *testing.T) {
	for _, tc := range []struct {
		Name                        string
		SecurityGroup               SecurityGroup
		GetSecurityGroupByNameCall  GetSecurityGroupByNameCall
		DeleteSecurityGroupByIDCall DeleteSecurityGroupByIDCall
		ExpectedError               error
	}{
		{
			Name: "happy case of delete by ID",
			SecurityGroup: SecurityGroup{
				GroupID: aws.String("groupID"),
			},
			DeleteSecurityGroupByIDCall: DeleteSecurityGroupByIDCall{
				GroupID: aws.String("groupID"),
				Err:     nil,
			},
		},
		{
			Name: "happy case of delete by name",
			SecurityGroup: SecurityGroup{
				GroupName: aws.String("groupName"),
			},
			GetSecurityGroupByNameCall: GetSecurityGroupByNameCall{
				GroupName: aws.String("groupName"),
				Instance: &ec2.SecurityGroup{
					GroupId:   aws.String("groupID"),
					GroupName: aws.String("groupName"),
				},
			},
			DeleteSecurityGroupByIDCall: DeleteSecurityGroupByIDCall{
				GroupID: aws.String("groupID"),
				Err:     nil,
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ec2 := &mocks.EC2API{}

			if tc.GetSecurityGroupByNameCall.GroupName != nil {
				ec2.On("GetVPCID").Return(aws.String("vpc-id"), nil)
				ec2.On("GetSecurityGroupByName", "vpc-id", *tc.GetSecurityGroupByNameCall.GroupName).Return(tc.GetSecurityGroupByNameCall.Instance, tc.GetSecurityGroupByNameCall.Err)
			}
			if tc.DeleteSecurityGroupByIDCall.GroupID != nil {
				ec2.On("DeleteSecurityGroupByID", aws.StringValue(tc.DeleteSecurityGroupByIDCall.GroupID)).Return(tc.DeleteSecurityGroupByIDCall.Err)
			}

			controller := &securityGroupController{
				ec2:    ec2,
				logger: log.New("test"),
			}
			err := controller.Delete(&tc.SecurityGroup)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			ec2.AssertExpectations(t)
		})
	}
}

func TestDiffIPPermissions(t *testing.T) {
	for _, tc := range []struct {
		source        []*ec2.IpPermission
		target        []*ec2.IpPermission
		expectedDiffs []*ec2.IpPermission
	}{
		{
			source: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
			target: []*ec2.IpPermission{},
			expectedDiffs: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
		},
		{
			source: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("udp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(8080),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(8081),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
			target: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
			expectedDiffs: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("udp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(8080),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(8081),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
		},
		{
			source: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
						{
							CidrIp: aws.String("192.168.1.2/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
						{
							CidrIp: aws.String("192.168.1.2/32"),
						},
						{
							CidrIp: aws.String("192.168.1.3/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
			target: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
						{
							CidrIp: aws.String("192.168.1.2/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
			expectedDiffs: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
						{
							CidrIp: aws.String("192.168.1.2/32"),
						},
						{
							CidrIp: aws.String("192.168.1.3/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
			},
		},
		{
			source: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
						{
							GroupId: aws.String("groupB"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
						{
							GroupId: aws.String("groupB"),
						},
						{
							GroupId: aws.String("groupC"),
						},
					},
				},
			},
			target: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
						{
							GroupId: aws.String("groupB"),
						},
					},
				},
			},
			expectedDiffs: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
					},
				},
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(81),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("192.168.1.1/32"),
						},
					},
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: aws.String("groupA"),
						},
						{
							GroupId: aws.String("groupB"),
						},
						{
							GroupId: aws.String("groupC"),
						},
					},
				},
			},
		},
	} {
		actualDiffs := diffIPPermissions(tc.source, tc.target)
		if !reflect.DeepEqual(tc.expectedDiffs, actualDiffs) {
			t.Errorf("expected:%v, actual %v", tc.expectedDiffs, actualDiffs)
		}
	}
}
