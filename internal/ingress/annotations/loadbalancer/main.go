/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package loadbalancer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albwaf"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

type PortData struct {
	Port   int64
	Scheme string
}

type Config struct {
	Scheme        *string
	IPAddressType *string
	WebACLId      *string

	InboundCidrs   util.Cidrs
	Ports          []PortData
	SecurityGroups util.AWSStringSlice
	Subnets        util.Subnets
	Attributes     albelbv2.LoadBalancerAttributes
}

type loadBalancer struct {
	r resolver.Resolver
}

const (
	DefaultIPAddressType = "ipv4"
)

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return loadBalancer{r}
}

// Parse parses the annotations contained in the resource
func (lb loadBalancer) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	// support legacy waf-acl-id annotation
	webACLId, _ := parser.GetStringAnnotation("waf-acl-id", ing)
	w, err := parser.GetStringAnnotation("web-acl-id", ing)
	if err == nil {
		if success, err := albwaf.WAFRegionalsvc.WebACLExists(w); !success {
			return nil, fmt.Errorf("Web ACL Id does not exist. Id: %s, error: %s", *w, err.Error())
		}
		webACLId = w
	}

	ipAddressType, err := parser.GetStringAnnotation("ip-address-type", ing)
	if err != nil {
		ipAddressType = aws.String(DefaultIPAddressType)
	}

	if *ipAddressType != "ipv4" && *ipAddressType != "dualstack" {
		return nil, errors.NewInvalidAnnotationContentReason("IP address type must be either `ipv4` or `dualstack`")
	}

	scheme, err := parser.GetStringAnnotation("scheme", ing)
	if err != nil {
		return nil, errors.NewInvalidAnnotationContentReason(`Necessary annotations missing. Must include scheme`)
	}

	if *scheme != "internal" && *scheme != "internet-facing" {
		return nil, errors.NewInvalidAnnotationContentReason("ALB scheme must be either `internal` or `internet-facing`")
	}
	// if config.RestrictScheme && *a.Scheme == "internet-facing" {
	// 	allowed := util.IngressAllowedExternal(config.RestrictSchemeNamespace, ingressNamespace, ingressName)
	// 	if !allowed {
	// 		return false
	// 	}
	// }
	// return true

	subnets, err := parseSubnets(ing, scheme)
	if err != nil {
		return nil, err
	}
	if len(subnets) == 0 {
		return nil, errors.NewInvalidAnnotationContentReason(`No subnets defined or were discoverable`)
	}

	ports, err := parsePorts(ing)
	if err != nil {
		return nil, err
	}

	attributes, err := parseAttributes(ing)
	if err != nil {
		return nil, err
	}

	securityGroups, err := parseSecurityGroups(ing)
	if err != nil {
		return nil, err
	}

	cidrs := util.Cidrs{}
	c, err := parser.GetStringAnnotation("security-group-inbound-cidrs", ing)
	if err == nil {
		for _, inboundCidr := range util.NewAWSStringSlice(*c) {
			cidrs = append(cidrs, inboundCidr)
		}
		// func (v ConcreteValidator) ValidateInboundCidrs(a *Annotations) error {
		// 	for _, cidr := range a.InboundCidrs {
		// 		ip, _, err := net.ParseCIDR(*cidr)
		// 		if err != nil {
		// 			return err
		// 		}

		// 		if ip.To4() == nil {
		// 			return fmt.Errorf("CIDR must use an IPv4 address: %v", *cidr)
		// 		}
		// 	}
		// }
	}

	return &Config{
		WebACLId:      webACLId,
		Scheme:        scheme,
		IPAddressType: ipAddressType,

		Attributes:   attributes,
		InboundCidrs: cidrs,
		Ports:        ports,

		Subnets:        subnets,
		SecurityGroups: securityGroups,
	}, nil
}

func parseSubnets(ing parser.AnnotationInterface, scheme *string) (util.Subnets, error) {
	v, err := parser.GetStringAnnotation("subnets", ing)
	// if the subnet annotation isn't specified, lookup appropriate subnets to use
	if err != nil {
		subnets, err := albec2.ClusterSubnets(scheme)
		return subnets, err
	}

	var names []*string
	var subnets util.AWSStringSlice

	for _, subnet := range util.NewAWSStringSlice(*v) {
		if strings.HasPrefix(*subnet, "subnet-") {
			subnets = append(subnets, subnet)
			continue
		}
		names = append(names, subnet)
	}

	if len(names) > 0 {
		nets, err := albec2.EC2svc.GetSubnets(names)
		if err != nil {
			return util.Subnets(subnets), err
		}

		subnets = append(subnets, nets...)
	}

	sort.Sort(subnets)
	if len(subnets) == 0 {
		return util.Subnets(subnets), fmt.Errorf("unable to resolve any subnets from: %s", *v)
	}
	// 	// Validate subnets
	// 		if err := validator.ResolveVPCValidateSubnets(a); err != nil {
	// 			return err
	// 		}
	// 	}

	return util.Subnets(subnets), nil

}

func parseAttributes(ing parser.AnnotationInterface) (albelbv2.LoadBalancerAttributes, error) {
	var badAttrs []string
	var lbattrs albelbv2.LoadBalancerAttributes

	attrs, _ := parser.GetStringAnnotation("attributes", ing)
	v, err := parser.GetStringAnnotation("load-balancer-attributes", ing)
	if err == nil {
		attrs = v
	}

	if attrs == nil {
		return nil, nil
	}

	rawAttrs := util.NewAWSStringSlice(*attrs)

	for _, rawAttr := range rawAttrs {
		parts := strings.Split(*rawAttr, "=")
		switch {
		case *rawAttr == "":
			continue
		case len(parts) != 2:
			badAttrs = append(badAttrs, *rawAttr)
			continue
		}
		lbattrs.Set(parts[0], parts[1])
	}

	if len(badAttrs) > 0 {
		return nil, fmt.Errorf("Unable to parse `%s` into Key=Value pair(s)", strings.Join(badAttrs, ", "))
	}
	return lbattrs, nil
}

// parsePorts takes a JSON array describing what ports and protocols should be used. When the JSON
// is empty, implying the annotation was not present, desired ports are set to the default. The
// default port value is 80 when a certArn is not present and 443 when it is.
func parsePorts(ing parser.AnnotationInterface) ([]PortData, error) {
	lps := []PortData{}
	p, err := parser.GetStringAnnotation("listen-ports", ing)
	if err != nil {
		// If port data is empty, default to port 80 or 443 contingent on whether a certArn was specified.
		_, err = parser.GetStringAnnotation("certificate-arn", ing)
		if err != nil {
			lps = append(lps, PortData{int64(80), "HTTP"})
		} else {
			lps = append(lps, PortData{int64(443), "HTTPS"})
		}
		return lps, nil
	}

	// Container to hold json in structured format after unmarshaling.
	c := []map[string]int64{}
	err = json.Unmarshal([]byte(*p), &c)
	if err != nil {
		return nil, fmt.Errorf("listen-ports JSON structure was invalid: %s", err.Error())
	}

	// Iterate over listeners in list. Validate port and protcol are correct, then inject them into
	// the list of ListenerPorts.
	for _, l := range c {
		for k, v := range l {
			// Verify port value is valid for ALB.
			// ALBS (from AWS): Ports need to be a number between 1 and 65535
			if v < 1 || v > 65535 {
				return nil, fmt.Errorf("Invalid port provided. Must be between 1 and 65535. It was %d", v)
			}
			switch {
			case k == "HTTP":
				lps = append(lps, PortData{v, k})
			case k == "HTTPS":
				lps = append(lps, PortData{v, k})
			default:
				return nil, fmt.Errorf("Invalid protocol provided. Must be HTTP or HTTPS and in order to use HTTPS you must have specified a certificate ARN")
			}
		}
	}

	return lps, nil
}

func parseSecurityGroups(ing parser.AnnotationInterface) (sgs util.AWSStringSlice, err error) {
	// no security groups specified means controller should manage them, if so return and sg will be
	// created and managed during reconcile.
	v, err := parser.GetStringAnnotation("security-groups", ing)
	if err != nil {
		return sgs, nil
	}

	var names []*string

	for _, sg := range util.NewAWSStringSlice(*v) {
		if strings.HasPrefix(*sg, "sg-") {
			sgs = append(sgs, sg)
			continue
		}

		names = append(names, sg)
	}

	if len(names) > 0 {
		groups, err := albec2.EC2svc.GetSecurityGroups(names)
		if err != nil {
			return sgs, err
		}

		sgs = append(sgs, groups...)
	}

	sort.Sort(sgs)
	if len(sgs) == 0 {
		return sgs, fmt.Errorf("unable to resolve any security groups from annotation containing: [%s]", *v)
	}

	// if c := cacheLookup(a.SecurityGroups.Hash()); c == nil || c.Expired() {
	// 	if err := validator.ValidateSecurityGroups(a); err != nil {
	// 		return err
	// 	}
	// 	cache.Set(a.SecurityGroups.Hash(), "success", 30*time.Minute)
	// }

	return sgs, nil
}
