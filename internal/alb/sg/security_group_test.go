package sg

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func TestReconcile(t *testing.T) {

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
