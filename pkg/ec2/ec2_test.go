package ec2

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/karlseguin/ccache"
)

func init() {
	mockedEC2responses = &mockedEC2ResponsesT{}
}

var (
	mockedEC2responses *mockedEC2ResponsesT
)

func TestGetVPCID(t *testing.T) {
	var (
		cache = ccache.New(ccache.Configure())
		svc = NewMockEC2(cache)
	)

	var tests = []struct {
		subnets               []*string
		vpc                   string
		err                   error
		DescribeSubnetsOutput *ec2.DescribeSubnetsOutput
	}{
		{
			[]*string{aws.String("subnet-abcdef")},
			"vpc-123456",
			nil,
			&ec2.DescribeSubnetsOutput{Subnets: []*ec2.Subnet{&ec2.Subnet{SubnetId: aws.String("subnet-abcdef"), VpcId: aws.String("vpc-123456")}}},
		},
		{
			[]*string{aws.String("subnet-abcdef")},
			"vpc-123456",
			fmt.Errorf(""),
			&ec2.DescribeSubnetsOutput{Subnets: []*ec2.Subnet{&ec2.Subnet{SubnetId: aws.String("subnet-abcdef"), VpcId: aws.String("vpc-999999")}}},
		},
		{
			[]*string{aws.String("subnet-abcdef")},
			"vpc-123456",
			fmt.Errorf(""),
			&ec2.DescribeSubnetsOutput{},
		},
		{
			[]*string{},
			"",
			fmt.Errorf("Empty subnet list provided to getVPCID"),
			&ec2.DescribeSubnetsOutput{},
		},
		{
			[]*string{aws.String("subnet-abcdef")},
			"",
			fmt.Errorf("DescribeSubnets returned no subnets"),
			&ec2.DescribeSubnetsOutput{Subnets: []*ec2.Subnet{}},
		},
	}

	for _, tt := range tests {
		cache.Clear()
		mockedEC2responses.DescribeSubnetsOutput = tt.DescribeSubnetsOutput
		mockedEC2responses.Error = tt.err

		vpc, err := svc.GetVPCID(tt.subnets)

		if tt.err == nil && err != nil {
			t.Errorf("getVPCID(%v) expected %s, got error: %v", awsutil.Prettify(tt.subnets), tt.vpc, err)
		}

		if tt.err != nil && err == nil {
			t.Errorf("getVPCID(%v): expected error (%s), but no error was returned", awsutil.Prettify(tt.subnets), tt.err)
		}

		if tt.err != nil && err != nil {
			if err.Error() == tt.err.Error() {
				continue
			} else {
				t.Errorf("getVPCID(%v): returned error (%s), expected error (%s)", awsutil.Prettify(tt.subnets), err, tt.err)

			}
		}

		if *vpc != tt.vpc {
			t.Errorf("getVPCID(%v) returned %v, expected %v", awsutil.Prettify(tt.subnets), *vpc, tt.vpc)
		}
	}
}
