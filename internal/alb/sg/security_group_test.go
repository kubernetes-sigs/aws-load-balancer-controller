package sg

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func TestReconcile(t *testing.T) {
	logger := log.New("test")
	for _, tc := range []struct {
		SecurityGroup              SecurityGroup
		GetSecurityGroupByIDFunc   func(string) (*ec2.SecurityGroup, error)
		GetSecurityGroupByNameFunc func(string, string) (*ec2.SecurityGroup, error)
		ExpectedError              *string
	}{
		{
			SecurityGroup: SecurityGroup{
				GroupID:            aws.String("groupID"),
				GroupName:          nil,
				InboundPermissions: nil,
			},
			GetSecurityGroupByIDFunc: func(string) (*ec2.SecurityGroup, error) { return nil, nil },
			ExpectedError:            aws.String("securityGroup groupID doesn't exist"),
		},
		{
			SecurityGroup: SecurityGroup{
				GroupID:   aws.String("groupID"),
				GroupName: nil,
				InboundPermissions: []*ec2.IpPermission{
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
								GroupId: aws.String("groupB"),
							},
						},
					},
				},
			},
			GetSecurityGroupByIDFunc: func(string) (*ec2.SecurityGroup, error) {
				return &ec2.SecurityGroup{
					GroupId:   aws.String("groupID"),
					GroupName: aws.String("groupName"),
					IpPermissions: []*ec2.IpPermission{
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
									GroupId: aws.String("groupC"),
								},
							},
						},
					},
				}, nil
			},
			ExpectedError: aws.String("securityGroup groupID doesn't exist"),
		},
	} {
		ec2 := albec2.NewMockEC2()
		ec2.GetSecurityGroupByIDFunc = tc.GetSecurityGroupByIDFunc
		ec2.GetSecurityGroupByNameFunc = tc.GetSecurityGroupByNameFunc
		controller := &securityGroupController{
			ec2:    ec2,
			logger: logger,
		}

		err := controller.Reconcile(&tc.SecurityGroup)

		if tc.ExpectedError != nil && (err == nil || !strings.Contains(err.Error(), *tc.ExpectedError)) {
			t.Errorf("expected error:%s, actual error:%v", *tc.ExpectedError, err)
		}
		if tc.ExpectedError == nil && err != nil {
			t.Errorf("expected nil error, actual error:%v", err)
		}
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
