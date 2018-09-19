package albec2

import (
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

// MockEC2 is mock implementation of EC2API
type MockEC2 struct {
	ec2iface.EC2API
	GetSubnetsFunc              func([]*string) ([]*string, error)
	GetSecurityGroupsFunc       func([]*string) ([]*string, error)
	GetVPCIDFunc                func() (*string, error)
	GetVPCFunc                  func(*string) (*ec2.Vpc, error)
	StatusFunc                  func() func() error
	IsNodeHealthyFunc           func(string) (bool, error)
	GetInstancesByIDsFunc       func([]string) ([]*ec2.Instance, error)
	GetSecurityGroupByIDFunc    func(string) (*ec2.SecurityGroup, error)
	GetSecurityGroupByNameFunc  func(string, string) (*ec2.SecurityGroup, error)
	DeleteSecurityGroupByIDFunc func(string) error
}

// GetSubnets is an mocked implementation
func (m *MockEC2) GetSubnets(taggedNames []*string) ([]*string, error) {
	return m.GetSubnetsFunc(taggedNames)
}

// GetSecurityGroups is an mocked implementation
func (m *MockEC2) GetSecurityGroups(taggedNames []*string) ([]*string, error) {
	return m.GetSecurityGroupsFunc(taggedNames)
}

// GetVPCID is an mocked implementation
func (m *MockEC2) GetVPCID() (*string, error) {
	return m.GetVPCIDFunc()
}

// GetVPC is an mocked implementation
func (m *MockEC2) GetVPC(vpcID *string) (*ec2.Vpc, error) {
	return m.GetVPCFunc(vpcID)
}

// Status is an mocked implementation
func (m *MockEC2) Status() func() error {
	return m.StatusFunc()
}

// IsNodeHealthy is an mocked implementation
func (m *MockEC2) IsNodeHealthy(instanceID string) (bool, error) {
	return m.IsNodeHealthyFunc(instanceID)
}

// GetInstancesByIDs is an mocked implementation
func (m *MockEC2) GetInstancesByIDs(instanceIDs []string) ([]*ec2.Instance, error) {
	return m.GetInstancesByIDs(instanceIDs)
}

// GetSecurityGroupByID is an mocked implementation
func (m *MockEC2) GetSecurityGroupByID(groupID string) (*ec2.SecurityGroup, error) {
	return m.GetSecurityGroupByIDFunc(groupID)
}

// GetSecurityGroupByName is an mocked implementation
func (m *MockEC2) GetSecurityGroupByName(vpcID string, groupName string) (*ec2.SecurityGroup, error) {
	return m.GetSecurityGroupByNameFunc(vpcID, groupName)
}

// DeleteSecurityGroupByID is an mocked implementation
func (m *MockEC2) DeleteSecurityGroupByID(groupID string) error {
	return m.DeleteSecurityGroupByID(groupID)
}
