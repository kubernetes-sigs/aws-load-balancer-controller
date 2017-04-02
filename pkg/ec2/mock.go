package ec2

import (
	aec2 "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/karlseguin/ccache"
)

func NewMockEC2(responses MockedEC2ResponsesT, cache *ccache.Cache) *EC2{
	svc := NewEC2(nil, nil, cache)
	svc.svc = &mockedEC2Client{responses: responses}
	return svc
}

type mockedEC2Client struct {
	ec2iface.EC2API
	responses MockedEC2ResponsesT
}

func (m *mockedEC2Client) DescribeSubnets(input *aec2.DescribeSubnetsInput) (*aec2.DescribeSubnetsOutput, error) {
	return m.responses.DescribeSubnetsOutput, m.responses.Error
}

func (m *mockedEC2Client) DescribeSecurityGroups(input *aec2.DescribeSecurityGroupsInput) (*aec2.DescribeSecurityGroupsOutput, error) {
	return m.responses.DescribeSecurityGroupsOutput, m.responses.Error
}

type MockedEC2ResponsesT struct {
	Error                        error
	DescribeSecurityGroupsOutput *aec2.DescribeSecurityGroupsOutput
	DescribeSubnetsOutput        *aec2.DescribeSubnetsOutput
}
