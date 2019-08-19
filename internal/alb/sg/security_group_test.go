package sg

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
)

type ReconcileEC2WithCurTagsCall struct {
	GroupID string
	Tags    map[string]string
	CurTags map[string]string
	Err     error
}

type RevokeSecurityGroupIngressCall struct {
	Input *ec2.RevokeSecurityGroupIngressInput
	Err   error
}

type AuthorizeSecurityGroupIngressCall struct {
	Input *ec2.AuthorizeSecurityGroupIngressInput
	Err   error
}

func TestReconcile(t *testing.T) {
	for _, tc := range []struct {
		Name               string
		Instance           ec2.SecurityGroup
		InboundPermissions []*ec2.IpPermission
		Tags               map[string]string

		ReconcileEC2WithCurTagsCall       *ReconcileEC2WithCurTagsCall
		RevokeSecurityGroupIngressCall    *RevokeSecurityGroupIngressCall
		AuthorizeSecurityGroupIngressCall *AuthorizeSecurityGroupIngressCall
		ExpectedError                     error
	}{
		{
			Name: "reconcile succeed without change anything",
			Instance: ec2.SecurityGroup{
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
				},
			},
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
			},
			Tags: map[string]string{},
			ReconcileEC2WithCurTagsCall: &ReconcileEC2WithCurTagsCall{
				GroupID: "groupID",
				Tags:    map[string]string{},
				CurTags: map[string]string{},
			},
		},
		{
			Name: "reconcile succeed by grant permissions",
			Instance: ec2.SecurityGroup{
				GroupId:   aws.String("groupID"),
				GroupName: aws.String("groupName"),
			},
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
			},
			Tags: map[string]string{},
			ReconcileEC2WithCurTagsCall: &ReconcileEC2WithCurTagsCall{
				GroupID: "groupID",
				Tags:    map[string]string{},
				CurTags: map[string]string{},
			},
			AuthorizeSecurityGroupIngressCall: &AuthorizeSecurityGroupIngressCall{
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
					},
				},
			},
		},
		{
			Name: "reconcile succeed by revoke permissions",
			Instance: ec2.SecurityGroup{
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
				},
			},
			Tags: map[string]string{},
			ReconcileEC2WithCurTagsCall: &ReconcileEC2WithCurTagsCall{
				GroupID: "groupID",
				Tags:    map[string]string{},
				CurTags: map[string]string{},
			},
			RevokeSecurityGroupIngressCall: &RevokeSecurityGroupIngressCall{
				Input: &ec2.RevokeSecurityGroupIngressInput{
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
					},
				},
			},
		},
		{
			Name: "reconcile succeed by grant and revoke permissions",
			Instance: ec2.SecurityGroup{
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
				},
			},
			InboundPermissions: []*ec2.IpPermission{
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
			Tags: map[string]string{},
			ReconcileEC2WithCurTagsCall: &ReconcileEC2WithCurTagsCall{
				GroupID: "groupID",
				Tags:    map[string]string{},
				CurTags: map[string]string{},
			},
			RevokeSecurityGroupIngressCall: &RevokeSecurityGroupIngressCall{
				Input: &ec2.RevokeSecurityGroupIngressInput{
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
					},
				},
			},
			AuthorizeSecurityGroupIngressCall: &AuthorizeSecurityGroupIngressCall{
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
			},
		},
		{
			Name: "reconcile failed when reconcile tags",
			Instance: ec2.SecurityGroup{
				GroupId:   aws.String("groupID"),
				GroupName: aws.String("groupName"),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("key1"),
						Value: aws.String("value1"),
					},
				},
			},
			Tags: map[string]string{"key1": "value2"},
			ReconcileEC2WithCurTagsCall: &ReconcileEC2WithCurTagsCall{
				GroupID: "groupID",
				Tags:    map[string]string{"key1": "value2"},
				CurTags: map[string]string{"key1": "value1"},
				Err:     errors.New("ReconcileEC2WithCurTagsCall"),
			},
			ExpectedError: errors.New("failed to reconcile tags due to ReconcileEC2WithCurTagsCall"),
		},
		{
			Name: "reconcile failed by grant permissions",
			Instance: ec2.SecurityGroup{
				GroupId:   aws.String("groupID"),
				GroupName: aws.String("groupName"),
			},
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
			},
			Tags: map[string]string{},
			ReconcileEC2WithCurTagsCall: &ReconcileEC2WithCurTagsCall{
				GroupID: "groupID",
				Tags:    map[string]string{},
				CurTags: map[string]string{},
			},
			AuthorizeSecurityGroupIngressCall: &AuthorizeSecurityGroupIngressCall{
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
					},
				},
				Err: errors.New("AuthorizeSecurityGroupIngressCall"),
			},
			ExpectedError: errors.New("failed to grant inbound permissions due to AuthorizeSecurityGroupIngressCall"),
		},
		{
			Name: "reconcile failed by revoke permissions",
			Instance: ec2.SecurityGroup{
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
				},
			},
			Tags: map[string]string{},
			ReconcileEC2WithCurTagsCall: &ReconcileEC2WithCurTagsCall{
				GroupID: "groupID",
				Tags:    map[string]string{},
				CurTags: map[string]string{},
			},
			RevokeSecurityGroupIngressCall: &RevokeSecurityGroupIngressCall{
				Input: &ec2.RevokeSecurityGroupIngressInput{
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
					},
				},
				Err: errors.New("RevokeSecurityGroupIngressCall"),
			},
			ExpectedError: errors.New("failed to revoke inbound permissions due to RevokeSecurityGroupIngressCall"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			tagsController := &tags.MockController{}
			if tc.ReconcileEC2WithCurTagsCall != nil {
				tagsController.On("ReconcileEC2WithCurTags", mock.Anything, tc.ReconcileEC2WithCurTagsCall.GroupID, tc.ReconcileEC2WithCurTagsCall.Tags, tc.ReconcileEC2WithCurTagsCall.CurTags).Return(tc.ReconcileEC2WithCurTagsCall.Err)
			}

			cloud := &mocks.CloudAPI{}
			if tc.AuthorizeSecurityGroupIngressCall != nil {
				cloud.On("AuthorizeSecurityGroupIngressWithContext", mock.Anything, tc.AuthorizeSecurityGroupIngressCall.Input).Return(nil, tc.AuthorizeSecurityGroupIngressCall.Err)
			}
			if tc.RevokeSecurityGroupIngressCall != nil {
				cloud.On("RevokeSecurityGroupIngressWithContext", mock.Anything, tc.RevokeSecurityGroupIngressCall.Input).Return(nil, tc.RevokeSecurityGroupIngressCall.Err)
			}

			sgController := securityGroupController{
				cloud:          cloud,
				tagsController: tagsController,
			}

			err := sgController.Reconcile(context.Background(), &tc.Instance, tc.InboundPermissions, tc.Tags)
			assert.Equal(t, err, tc.ExpectedError)
			tagsController.AssertExpectations(t)
			cloud.AssertExpectations(t)
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
					Ipv6Ranges: []*ec2.Ipv6Range{
						{
							CidrIpv6: aws.String("::/0"),
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
					Ipv6Ranges: []*ec2.Ipv6Range{
						{
							CidrIpv6: aws.String("::/0"),
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
					Ipv6Ranges: []*ec2.Ipv6Range{
						{
							CidrIpv6: aws.String("::/0"),
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
