package controller

import (
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/karlseguin/ccache"
)

type MockedR53ResponsesT struct {
	Error                          error
	ChangeResourceRecordSetsOutput *route53.ChangeResourceRecordSetsOutput
	ListHostedZonesByNameOutput    *route53.ListHostedZonesByNameOutput
	ListHostedZonesOutput          *route53.ListHostedZonesOutput
	ListResourceRecordSetsOutput   *route53.ListResourceRecordSetsOutput
}

func NewMockRoute53(responses *MockedR53ResponsesT, cache *ccache.Cache) *Route53 {
	mockedR53 := NewRoute53(nil, nil, cache)
	mockedR53.svc = &MockedRoute53Client{responses: responses}

	return mockedR53
	//goodListResourceRecordSetsOutput = &route53.ListResourceRecordSetsOutput{
	//	ResourceRecordSets: []*route53.ResourceRecordSet{
	//		&route53.ResourceRecordSet{
	//			Name: aws.String(hostname + "."),
	//		},
	//	},
	//}
	//emptyListResourceRecordSetsOutput = &route53.ListResourceRecordSetsOutput{
	//	ResourceRecordSets: []*route53.ResourceRecordSet{},
	//}
	//goodListHostedZonesByNameOutput = &route53.ListHostedZonesByNameOutput{
	//	HostedZones: []*route53.HostedZone{
	//		&route53.HostedZone{
	//			Id:   aws.String("/hostedzone/" + zoneID),
	//			Name: aws.String(zone + "."),
	//		},
	//	},
	//}
	//emptyListHostedZonesByNameOutput = &route53.ListHostedZonesByNameOutput{
	//	HostedZones: []*route53.HostedZone{},
	//}
	//
	//goodListHostedZonesOutput = &route53.ListHostedZonesOutput{}
	//
	//goodChangeResourceRecordSetsOutput = &route53.ChangeResourceRecordSetsOutput{}
}

type MockedRoute53Client struct {
	route53iface.Route53API
	responses *MockedR53ResponsesT
}

func (m *MockedRoute53Client) ListHostedZonesByName(input *route53.ListHostedZonesByNameInput) (*route53.ListHostedZonesByNameOutput, error) {
	return m.responses.ListHostedZonesByNameOutput, m.responses.Error
}

func (m *MockedRoute53Client) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return m.responses.ListResourceRecordSetsOutput, m.responses.Error
}

func (m *MockedRoute53Client) ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	return m.responses.ListHostedZonesOutput, m.responses.Error
}

func (m *MockedRoute53Client) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	return m.responses.ChangeResourceRecordSetsOutput, m.responses.Error
}
