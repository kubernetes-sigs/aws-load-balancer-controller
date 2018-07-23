package albrgt

import (
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
)

type Dummy struct {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
	resp      interface{}
	respError error
}

func (d *Dummy) GetClusterResources() (*Resources, error) {
	return d.resp.(*Resources), d.respError
}

func (d *Dummy) SetResponse(i interface{}, e error) {
	d.resp = i
	d.respError = e
}
