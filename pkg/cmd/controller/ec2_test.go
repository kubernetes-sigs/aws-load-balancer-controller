package controller

import (
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

var (
	mockEC2      *EC2
	ec2responses map[string]interface{}
)

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
