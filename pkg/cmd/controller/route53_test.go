package controller

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

var (
	r53       *Route53
	responses map[string]interface{}
)

const hostname = "2048.nonprod-tmaws.io"
const zone = "nonprod-tmaws.io"
const zoneID = "Z3QX9G7OLI3M7W"

func TestLookupRecord(t *testing.T) {
	setup()

	var tests = []struct {
		hostname                     string
		pass                         bool
		listHostedZonesByNameOutput  *route53.ListHostedZonesByNameOutput
		listResourceRecordSetsOutput *route53.ListResourceRecordSetsOutput
	}{
		{hostname, true, goodListHostedZonesByNameOutput, goodListResourceRecordSetsOutput},
		{hostname, false, goodListHostedZonesByNameOutput, emptyListResourceRecordSetsOutput},
		{"", false, goodListHostedZonesByNameOutput, emptyListResourceRecordSetsOutput},
	}

	for _, tt := range tests {
		responses["ListHostedZonesByNameOutput"] = tt.listHostedZonesByNameOutput
		responses["ListResourceRecordSetsOutput"] = tt.listResourceRecordSetsOutput

		record, err := r53.lookupRecord(tt.hostname)
		if tt.pass == false && err != nil {
			continue
		}
		if err != nil && tt.pass {
			t.Errorf("lookupRecord(%v): expected %v, got error: %s", tt.hostname, tt.hostname, err)
		}
		if err == nil && !tt.pass {
			t.Errorf("lookupRecord(%v): expected %v, did not get error", tt.hostname, tt.pass)
		}
		if *record.Name != hostname && *record.Name != hostname+"." {
			t.Errorf("lookupRecord(%v): expected %v, actual %s", tt.hostname, tt.hostname, *record.Name)
		}
	}
}

func TestGetDomain(t *testing.T) {
	setup()

	var tests = []struct {
		hostname string
		domain   string
		pass     bool
	}{
		{"2048.domain.io", "domain.io", true},
		{"alpha.beta.domain.io", "domain.io", true},
		{"", "", false},
	}

	for _, tt := range tests {
		actual, err := r53.getDomain(tt.hostname)
		if tt.pass == false && err != nil {
			continue
		}
		if err != nil {
			t.Errorf("getDomain(%s): expected %s, got error: %s", tt.hostname, tt.domain, err)
		}
		if actual != tt.domain {
			t.Errorf("getDomain(%s): expected %s, actual %s", tt.hostname, tt.domain, actual)
		}
	}
}

func TestGetZone(t *testing.T) {
	setup()

	var tests = []struct {
		zone                        string
		pass                        bool
		listHostedZonesByNameOutput *route53.ListHostedZonesByNameOutput
	}{
		{zone, true, goodListHostedZonesByNameOutput},
		{zone + ".bad", false, goodListHostedZonesByNameOutput},
		{zone, false, emptyListHostedZonesByNameOutput},
	}

	for _, tt := range tests {
		responses["ListHostedZonesByNameOutput"] = tt.listHostedZonesByNameOutput

		actual, err := r53.getZoneID(tt.zone)
		if err != nil && tt.pass == false {
			continue
		}
		if err != nil {
			t.Errorf("getZoneID(%s): expected %v, got error: %s", tt.zone, tt.pass, err)
		}
		if *actual.Name != tt.zone && *actual.Name != tt.zone+"." {
			t.Errorf("getZoneID(%s): expected %v, actual %v", tt.zone, tt.pass, *actual.Name)
		}
	}
}

func TestModifyRecord(t *testing.T) {
	setup()

	var tests = []struct {
		hostname                       string
		target                         string
		targetZoneID                   string
		action                         string
		pass                           bool
		changeResourceRecordSetsOutput *route53.ChangeResourceRecordSetsOutput
	}{
		{hostname, "target.com", "ZZZZZZZZZZZZZZ", "UPSERT", true, goodChangeResourceRecordSetsOutput},
	}

	for _, tt := range tests {
		responses["ListHostedZonesByNameOutput"] = goodListHostedZonesByNameOutput
		responses["ChangeResourceRecordSetsOutput"] = tt.changeResourceRecordSetsOutput
		alb := &albIngress{
			hostname: tt.hostname,
			elbv2: &ELBV2{
				LoadBalancer: &elbv2.LoadBalancer{DNSName: aws.String(tt.target)},
			},
		}
		alb.elbv2.CanonicalHostedZoneId = aws.String(tt.targetZoneID)
		err := r53.modifyRecord(alb, tt.action)
		if tt.pass == false && err != nil {
			continue
		}
		if tt.pass == false && err == nil {
			t.Errorf("modifyRecord(%v, %v) expected %v, did not error", alb, tt.action, tt.pass)
		}
		if err != nil && tt.pass {
			t.Errorf("modifyRecord(%v, %v) expected %v, got error: %s", alb, tt.action, tt.pass, err)
		}
	}
}

func TestSanityTest(t *testing.T) {
	setup()
	responses["ListHostedZonesOutput"] = goodListHostedZonesOutput
	r53.sanityTest()
}

var (
	goodListResourceRecordSetsOutput  *route53.ListResourceRecordSetsOutput
	emptyListResourceRecordSetsOutput *route53.ListResourceRecordSetsOutput

	goodListHostedZonesByNameOutput  *route53.ListHostedZonesByNameOutput
	emptyListHostedZonesByNameOutput *route53.ListHostedZonesByNameOutput

	goodListHostedZonesOutput *route53.ListHostedZonesOutput

	goodChangeResourceRecordSetsOutput *route53.ChangeResourceRecordSetsOutput
)

func setupReal() {
	r53 = newRoute53(nil)
}

func setup() {
	r53 = newRoute53(nil)
	r53.svc = &mockRoute53Client{}
	responses = make(map[string]interface{})

	goodListResourceRecordSetsOutput = &route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: []*route53.ResourceRecordSet{
			&route53.ResourceRecordSet{
				Name: aws.String(hostname + "."),
			},
		},
	}
	emptyListResourceRecordSetsOutput = &route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: []*route53.ResourceRecordSet{},
	}
	goodListHostedZonesByNameOutput = &route53.ListHostedZonesByNameOutput{
		HostedZones: []*route53.HostedZone{
			&route53.HostedZone{
				Id:   aws.String("/hostedzone/" + zoneID),
				Name: aws.String(zone + "."),
			},
		},
	}
	emptyListHostedZonesByNameOutput = &route53.ListHostedZonesByNameOutput{
		HostedZones: []*route53.HostedZone{},
	}

	goodListHostedZonesOutput = &route53.ListHostedZonesOutput{}

	goodChangeResourceRecordSetsOutput = &route53.ChangeResourceRecordSetsOutput{}
}

type mockRoute53Client struct {
	route53iface.Route53API
}

func (m *mockRoute53Client) ListHostedZonesByName(input *route53.ListHostedZonesByNameInput) (*route53.ListHostedZonesByNameOutput, error) {
	return responses["ListHostedZonesByNameOutput"].(*route53.ListHostedZonesByNameOutput), nil
}

func (m *mockRoute53Client) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return responses["ListResourceRecordSetsOutput"].(*route53.ListResourceRecordSetsOutput), nil
}

func (m *mockRoute53Client) ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	return responses["ListHostedZonesOutput"].(*route53.ListHostedZonesOutput), nil
}

func (m *mockRoute53Client) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	return responses["ChangeResourceRecordSetsOutput"].(*route53.ChangeResourceRecordSetsOutput), nil
}
