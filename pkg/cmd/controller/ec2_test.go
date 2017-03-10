package controller

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

var (
	mockEC2      *EC2
	ec2responses map[string]interface{}
)

func TestGetVPCID(t *testing.T) {
	setup()

	var tests = []struct {
		subnet                string
		vpc                   string
		pass                  bool
		DescribeSubnetsOutput *ec2.DescribeSubnetsOutput
	}{
		{"subnet-abcdef", "vpc-123456", true, &ec2.DescribeSubnetsOutput{Subnets: []*ec2.Subnet{&ec2.Subnet{SubnetId: aws.String("subnet-abcdef"), VpcId: aws.String("vpc-123456")}}}},
		{"subnet-abcdef", "vpc-123456", false, &ec2.DescribeSubnetsOutput{Subnets: []*ec2.Subnet{&ec2.Subnet{SubnetId: aws.String("subnet-abcdef"), VpcId: aws.String("vpc-999999")}}}},
		{"subnet-abcdef", "vpc-123456", false, &ec2.DescribeSubnetsOutput{}},
	}

	for _, tt := range tests {
		ec2responses["DescribeSubnets"] = tt.DescribeSubnetsOutput
		subnets := []*string{aws.String(tt.subnet)}
		vpc, err := mockEC2.getVPCID(subnets)
		if err != nil && tt.pass {
			t.Errorf("getVPCID(%v) failed: %v", awsutil.Prettify(subnets), err)
		}
		if err != nil && !tt.pass {
			continue
		}
		if *vpc != tt.vpc && tt.pass {
			t.Errorf("getVPCID(%v) returned %v, expected %v", awsutil.Prettify(subnets), *vpc, tt.vpc)
		}
		if *vpc == tt.vpc && !tt.pass {
			t.Errorf("getVPCID(%v) returned %v but should not match %v", awsutil.Prettify(subnets), *vpc, tt.vpc)
		}
	}
}

func setupEC2() {
	mockEC2 = newEC2(nil)
	mockEC2.svc = &mockEC2Client{}
	ec2responses = make(map[string]interface{})
}

type mockEC2Client struct {
	ec2iface.EC2API
}

func (m *mockEC2Client) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	return ec2responses["DescribeSubnets"].(*ec2.DescribeSubnetsOutput), nil
}

func (m *mockEC2Client) DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	return ec2responses["DescribeSecurityGroups"].(*ec2.DescribeSecurityGroupsOutput), nil
}
