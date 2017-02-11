package controller

import (
	"fmt"
	"strings"

	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

type Route53 struct {
	*route53.Route53
}

func newRoute53(awsconfig *aws.Config) *Route53 {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	r53 := Route53{
		route53.New(session),
	}
	return &r53
}

// getDomain looks for the 'domain' of the hostname
// This will never work if people define zones for subdomains
// We can either search better or we can require a zone id in the
// ingress annotations
func (r *Route53) getDomain(hostname string) (string, error) {
	hostname = strings.TrimSuffix(hostname, ".")
	domainParts := strings.Split(hostname, ".")
	if len(domainParts) < 2 {
		return "", fmt.Errorf("%s hostname does not contain a domain", hostname)
	}

	domain := strings.Join(domainParts[len(domainParts)-2:], ".")

	return strings.ToLower(domain), nil
}

// getZoneID looks for the Route53 zone ID of the hostname passed to it
// some voodoo is involved when stripping the domain
func (r *Route53) getZoneID(hostname string) (*route53.HostedZone, error) {
	zone, err := r.getDomain(hostname)
	if err != nil {
		return nil, err
	}

	glog.Infof("Fetching Zones matching %s", zone)
	resp, err := r.ListHostedZonesByName(
		&route53.ListHostedZonesByNameInput{
			DNSName: aws.String(zone),
		})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", awsErr.Code())
		}
		return nil, fmt.Errorf("Error calling route53.ListHostedZonesByName: %s", err)
	}

	if len(resp.HostedZones) == 0 {
		glog.Errorf("Unable to find the %s zone in Route53", zone)
		return nil, fmt.Errorf("Zone not found")
	}

	for _, i := range resp.HostedZones {
		zoneName := strings.TrimSuffix(*i.Name, ".")
		if zone == zoneName {
			glog.Infof("Found DNS Zone %s with ID %s", zoneName, *i.Id)
			return i, nil
		}
	}
	return nil, fmt.Errorf("Unable to find the zone: %s", zone)
}

func (r *Route53) upsertRecord(alb *albIngress) error {
	hostedZone, err := r.getZoneID(alb.hostname)
	if err != nil {
		return err
	}

	// Alias record set looks something like this
	// resourceRecordSet := &route53.ResourceRecordSet{
	// 	Name: aws.String(alb.hostname),
	// 	Type: aws.String("A"),
	// 	AliasTarget: &route53.AliasTarget{
	// 		DNSName:              aws.String(alb.alb.???),
	// 		EvaluateTargetHealth: aws.Bool(false),
	// 		HostedZoneId:         aws.String(target zone id),
	// 	},
	// },

	resourceRecordSet := &route53.ResourceRecordSet{
		Name: aws.String(alb.hostname),
		Type: aws.String("CNAME"),
		ResourceRecords: []*route53.ResourceRecord{
			&route53.ResourceRecord{
				// TODO:
				Value: aws.String("alb.alb.???"),
			},
		},
		TTL: aws.Int64(60),
	}

	changes := []*route53.Change{&route53.Change{
		Action:            aws.String("UPSERT"),
		ResourceRecordSet: resourceRecordSet,
	}}

	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: changes,
			Comment: aws.String(fmt.Sprintf("KUBERNETES:%s", alb.clusterName)),
		},
		HostedZoneId: hostedZone.Id,
	}

	resp, err := r.ChangeResourceRecordSets(params)
	if err != nil {
		glog.Errorf("There was an Error calling Route53 ChangeResourceRecordSets: %+v. Error: %s", resp.GoString(), err.Error())
		return err
	}

	glog.Infof("Successfully registered %s in Route53", alb.hostname)
	return nil
}

func (r *Route53) sanityTest() {
	glog.Warning("Verifying Route53 connectivity") // TODO: Figure out why we can't see this
	_, err := r.ListHostedZones(&route53.ListHostedZonesInput{MaxItems: aws.String("1")})
	if err != nil {
		panic(err)
	}
	glog.Warning("Verified")
}
