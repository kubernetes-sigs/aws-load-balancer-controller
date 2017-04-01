package ec2 

import (
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)


func NewEC2() {
	ec2svc = ec2.NewEC2(nil, )
	ec2svc.svc = &mockedEC2Client{}
}

type mockedEC2Client struct {
	ec2iface.EC2API
}

func (m *mockedEC2Client) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	return mockedEC2responses.DescribeSubnetsOutput, mockedEC2responses.Error
}

func (m *mockedEC2Client) DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	return mockedEC2responses.DescribeSecurityGroupsOutput, mockedEC2responses.Error
}

type mockedEC2ResponsesT struct {
	Error                        error
	DescribeSecurityGroupsOutput *ec2.DescribeSecurityGroupsOutput
	DescribeSubnetsOutput        *ec2.DescribeSubnetsOutput
}
