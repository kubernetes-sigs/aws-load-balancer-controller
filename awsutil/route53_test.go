package awsutil

import (
	//"fmt"
	//"testing"

	//"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	//"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

type mockedR53ResponsesT struct {
	Error                          error
	ChangeResourceRecordSetsOutput *route53.ChangeResourceRecordSetsOutput
	ListHostedZonesByNameOutput    *route53.ListHostedZonesByNameOutput
	ListHostedZonesOutput          *route53.ListHostedZonesOutput
	ListResourceRecordSetsOutput   *route53.ListResourceRecordSetsOutput
}

var (
	mockedR53          *Route53
	mockedR53responses *mockedR53ResponsesT
)

//func TestLookupRecord(t *testing.T) {
//	setup()
//
//	var tests = []struct {
//		hostname                     string
//		pass                         bool
//		listHostedZonesByNameOutput  *route53.ListHostedZonesByNameOutput
//		listResourceRecordSetsOutput *route53.ListResourceRecordSetsOutput
//	}{}
//
//	ac := NewALBController(nil, nil)
//	ing := &extensions.Ingress{
//		Spec: extensions.IngressSpec{
//			Backend: &extensions.IngressBackend{
//				ServiceName: "Service1",
//				ServicePort: intstr.IntOrString{StrVal: "80"},
//			},
//		},
//	}
//
//	a := newAlbIngressesFromIngress(ing, ac)
//
//	fmt.Printf("%s%s", a, tests)
//
//
//}

/*func TestGetDomain(t *testing.T) {
	setup()

	var tests = []struct {
		hostname string
		domain   string
		err      error
	}{
		{"beta.domain.io", "domain.io", nil},
		{"alpha.beta.domain.io", "domain.io", nil},
		{"", "", fmt.Errorf(" hostname does not contain a domain")},
	}

	for _, tt := range tests {
		actual, err := mockedR53.getDomain(tt.hostname)
		if tt.err == nil && err != nil {
			t.Errorf("getDomain(%s): expected %s, got error: %s", tt.hostname, tt.domain, err)
		}
		if tt.err != nil && err == nil {
			t.Errorf("getDomain(%s): expected error (%s), but no error was returned", tt.hostname, tt.err)
		}
		if tt.err != nil && err != nil {
			if tt.err.Error() == err.Error() {
				continue
			} else {
				t.Errorf("getDomain(%s): returned error (%s), expected error (%s)", tt.hostname, err, tt.err)
			}
		}
		if *actual != tt.domain {
			t.Errorf("getDomain(%s): expected %s, actual %s", tt.hostname, tt.domain, actual)
		}
	}
}*/

/*func TestDescribeResourceRecordSets(t *testing.T) {
	setup()

	var tests = []struct {
		zoneID                       string
		hostname                     string
		err                          error
		ListResourceRecordSetsOutput *route53.ListResourceRecordSetsOutput
	}{
		{
			"ZZZZZZZZZZZZZZ",
			"beta.domain.io",
			fmt.Errorf(""),
			&route53.ListResourceRecordSetsOutput{ResourceRecordSets: []*route53.ResourceRecordSet{}},
		},
		{
			"ZZZZZZZZZZZZZZ",
			"beta.domain.io",
			nil,
			&route53.ListResourceRecordSetsOutput{ResourceRecordSets: []*route53.ResourceRecordSet{
				&route53.ResourceRecordSet{},
			}},
		},
	}
	for _, tt := range tests {
		cache.Clear()
		mockedR53responses.ListResourceRecordSetsOutput = tt.ListResourceRecordSetsOutput
		mockedR53responses.Error = tt.err

		_, err := mockedR53.DescribeResourceRecordSets(aws.String(tt.zoneID), aws.String(tt.hostname))
		if tt.err == nil && err != nil {
			t.Errorf("DescribeResourceRecordSets(%s, %s): got error: %s", tt.zoneID, tt.hostname, err)
		}
		if tt.err != nil && err == nil {
			t.Errorf("DescribeResourceRecordSets(%s, %s): expected error (%s), but no error was returned", tt.zoneID, tt.hostname, tt.err)
		}
		if tt.err != nil && err != nil {
			if tt.err.Error() == err.Error() {
				continue
			} else {
				t.Errorf("DescribeResourceRecordSets(%s, %s): returned error (%s), expected error (%s)", tt.zoneID, tt.hostname, err, tt.err)
			}
		}
	}
}*/

// Domain should return appropriate Zone, and specifically ZoneID. Cache is cleared on every iteration.
/*func TestGetZone(t *testing.T) {
	setup()

	var tests = []struct {
		hostname                    string
		pass                        bool
		expected                    string
		listHostedZonesByNameOutput *route53.ListHostedZonesByNameOutput
	}{
		{"beta.domain.io", true, "Z37F6OLDZZO6IG", &route53.ListHostedZonesByNameOutput{
			HostedZones: []*route53.HostedZone{
				{Id: aws.String("Z37F6OLDZZO6IG"), Name: aws.String("domain.io")},
			},
		}},
		{"some.random.com", true, "Z12VUW31WR31HD", &route53.ListHostedZonesByNameOutput{
			HostedZones: []*route53.HostedZone{
				{Id: aws.String("Z12VUW31WR31HD"), Name: aws.String("random.com")},
			},
		}},
		// Should fail due to zone not existing
		{"beta.new.com", false, "", &route53.ListHostedZonesByNameOutput{
			HostedZones: []*route53.HostedZone{
				{Id: aws.String("Z12VUW31WR31HD"), Name: aws.String("domain.io")},
			},
		}},
	}

	for _, tt := range tests {
		cache.Clear()
		mockedR53responses.ListHostedZonesByNameOutput = tt.listHostedZonesByNameOutput
		actual, err := mockedR53.GetZoneID(&tt.hostname)
		switch {
		// expected error
		case tt.pass == false && err != nil:
			break
		// expected success
		case err == nil && tt.expected == *actual.Id:
			break
		// test failure due to unexpected error
		case err != nil:
			t.Errorf("GetZoneID(%s): expected %s, got error: %s", tt.hostname, tt.expected, err)
		// test failure due to value mismatch
		case *actual.Id != tt.expected:
			t.Errorf("GetZoneID(%s): expected %s, actual %s", tt.hostname, tt.expected, *actual.Id)
		default:
			t.Errorf("GetZoneID(%s): experienced test failure for an unknown reason", tt.hostname)
		}
	}
}*/

// Domain should be cached based on full hostname - subdomain. For example, beta.domain.io should cache domain.io
// hosted zone results. Meaning alpha.domain.io resolves the same hostedZoneId without needing to call
// route53.listHostedZonesByName again.
/*func TestGetZoneCache(t *testing.T) {
	setup()

	var tests = []struct {
		hostname                    string
		pass                        bool
		expected                    string
		listHostedZonesByNameOutput *route53.ListHostedZonesByNameOutput
	}{
		{"beta.domain.io", true, "Z37F6OLDZZO6IG", &route53.ListHostedZonesByNameOutput{
			HostedZones: []*route53.HostedZone{
				{Id: aws.String("Z37F6OLDZZO6IG"), Name: aws.String("domain.io")},
			},
		}},
		{"alpha.domain.io", true, "Z37F6OLDZZO6IG", &route53.ListHostedZonesByNameOutput{}},
		{"stable.domain.io", true, "Z37F6OLDZZO6IG", &route53.ListHostedZonesByNameOutput{}},
		// should throw zone not found
		{"stable.different.io", false, "", &route53.ListHostedZonesByNameOutput{}},
	}

	for _, tt := range tests {
		mockedR53responses.ListHostedZonesByNameOutput = tt.listHostedZonesByNameOutput
		actual, err := mockedR53.GetZoneID(&tt.hostname)

		switch {
		// expected error
		case tt.pass == false && err != nil:
			break
		// expected success
		case err == nil && tt.expected == *actual.Id:
			break
		// test failure due to unexpected error
		case err != nil:
			t.Errorf("GetZoneID(%s): expected %s, got error: %s", tt.hostname, tt.expected, err)
		// test failure due to value mismatch
		case *actual.Id != tt.expected:
			t.Errorf("GetZoneID(%s): expected %s, actual %s", tt.hostname, tt.expected, *actual.Id)
		default:
			t.Errorf("GetZoneID(%s): experienced test failure for an unknown reason", tt.hostname)
		}
	}
}*/

// func TestModifyRecord(t *testing.T) {
// 	setup()

// 	var tests = []struct {
// 		hostname                       string
// 		target                         string
// 		targetZoneID                   string
// 		action                         string
// 		pass                           bool
// 		changeResourceRecordSetsOutput *route53.ChangeResourceRecordSetsOutput
// 	}{
// 		{hostname, "target.com", "ZZZZZZZZZZZZZZ", "UPSERT", true, goodChangeResourceRecordSetsOutput},
// 	}

// 	for _, tt := range tests {
// 		r53responses["ListHostedZonesByName"] = goodListHostedZonesByNameOutput
// 		r53responses["ChangeResourceRecordSets"] = tt.changeResourceRecordSetsOutput
// 		alb := &ALBIngress{
// 			hostname:              tt.hostname,
// 			loadBalancerDNSName:   tt.target,
// 			canonicalHostedZoneId: tt.targetZoneID,
// 		}

// 		err := r53.modify(alb, tt.action)
// 		if tt.pass == false && err != nil {
// 			continue
// 		}
// 		if tt.pass == false && err == nil {
// 			t.Errorf("modify(%v, %v) expected %v, did not error", alb, tt.action, tt.pass)
// 		}
// 		if err != nil && tt.pass {
// 			t.Errorf("modify(%v, %v) expected %v, got error: %s", alb, tt.action, tt.pass, err)
// 		}
// 	}
// }

/*func setupRoute53() {
	mockedR53 = newRoute53(nil)
	mockedR53.Svc = &mockedRoute53Client{}
	mockedR53responses = &mockedR53ResponsesT{}

	//goodListResourceRecordSetsOutput = &route53.ListResourceRecordSetsOutput{
	//	ResourceRecordSets: []*route53.CurrentResourceRecordSet{
	//		&route53.CurrentResourceRecordSet{
	//			Name: aws.String(hostname + "."),
	//		},
	//	},
	//}
	//emptyListResourceRecordSetsOutput = &route53.ListResourceRecordSetsOutput{
	//	ResourceRecordSets: []*route53.CurrentResourceRecordSet{},
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
}*/

/*type mockedRoute53Client struct {
	route53iface.Route53API
}

func (m *mockedRoute53Client) ListHostedZonesByName(input *route53.ListHostedZonesByNameInput) (*route53.ListHostedZonesByNameOutput, error) {
	return mockedR53responses.ListHostedZonesByNameOutput, mockedR53responses.Error
}

func (m *mockedRoute53Client) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return mockedR53responses.ListResourceRecordSetsOutput, mockedR53responses.Error
}

func (m *mockedRoute53Client) ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	return mockedR53responses.ListHostedZonesOutput, mockedR53responses.Error
}

func (m *mockedRoute53Client) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	return mockedR53responses.ChangeResourceRecordSetsOutput, mockedR53responses.Error
}
*/