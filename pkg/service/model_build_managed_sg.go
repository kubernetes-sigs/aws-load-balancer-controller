package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

const (
	resourceIDManagedSecurityGroup = "ManagedLBSecurityGroup"
)

func (t *defaultModelBuildTask) buildManagedSecurityGroup(ctx context.Context, ipAddressType elbv2model.IPAddressType) (*ec2model.SecurityGroup, error) {
	sgSpec, err := t.buildManagedSecurityGroupSpec(ctx, ipAddressType)
	if err != nil {
		return nil, err
	}
	sg := ec2model.NewSecurityGroup(t.stack, resourceIDManagedSecurityGroup, sgSpec)
	return sg, nil
}

func (t *defaultModelBuildTask) buildManagedSecurityGroupSpec(ctx context.Context, ipAddressType elbv2model.IPAddressType) (ec2model.SecurityGroupSpec, error) {
	name := t.buildManagedSecurityGroupName(ctx)
	tags, err := t.buildManagedSecurityGroupTags(ctx)
	if err != nil {
		return ec2model.SecurityGroupSpec{}, err
	}
	ingressPermissions, err := t.buildManagedSecurityGroupIngressPermissions(ctx, ipAddressType)
	if err != nil {
		return ec2model.SecurityGroupSpec{}, err
	}
	return ec2model.SecurityGroupSpec{
		GroupName:   name,
		Description: "[k8s] Managed SecurityGroup for LoadBalancer",
		Tags:        tags,
		Ingress:     ingressPermissions,
	}, nil
}

var invalidSecurityGroupNamePtn, _ = regexp.Compile("[[:^alnum:]]")

func (t *defaultModelBuildTask) buildManagedSecurityGroupName(_ context.Context) string {
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.service.Name))
	_, _ = uuidHash.Write([]byte(t.service.Namespace))
	_, _ = uuidHash.Write([]byte(t.service.UID))

	uuid := hex.EncodeToString(uuidHash.Sum(nil))
	sanitizedName := invalidSecurityGroupNamePtn.ReplaceAllString(t.service.Name, "")
	sanitizedNamespace := invalidSecurityGroupNamePtn.ReplaceAllString(t.service.Namespace, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildManagedSecurityGroupIngressPermissions(ctx context.Context, ipAddressType elbv2model.IPAddressType) ([]ec2model.IPPermission, error) {
	var permissions []ec2model.IPPermission
	var prefixListIDs []string
	prefixListsConfigured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSecurityGroupPrefixLists, &prefixListIDs, t.service.Annotations)
	cidrs, err := t.buildCIDRsFromSourceRanges(ctx, ipAddressType, prefixListsConfigured)
	if err != nil {
		return nil, err
	}
	for _, port := range t.service.Spec.Ports {
		listenPort := int64(port.Port)
		for _, cidr := range cidrs {
			if !strings.Contains(cidr, ":") {
				permissions = append(permissions, ec2model.IPPermission{
					IPProtocol: strings.ToLower(string(port.Protocol)),
					FromPort:   awssdk.Int64(listenPort),
					ToPort:     awssdk.Int64(listenPort),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: cidr,
						},
					},
				})
			} else {
				permissions = append(permissions, ec2model.IPPermission{
					IPProtocol: strings.ToLower(string(port.Protocol)),
					FromPort:   awssdk.Int64(listenPort),
					ToPort:     awssdk.Int64(listenPort),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: cidr,
						},
					},
				})
			}
		}
		if prefixListsConfigured {
			for _, prefixID := range prefixListIDs {
				permissions = append(permissions, ec2model.IPPermission{
					IPProtocol: strings.ToLower(string(port.Protocol)),
					FromPort:   awssdk.Int64(listenPort),
					ToPort:     awssdk.Int64(listenPort),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: prefixID,
						},
					},
				})
			}
		}
	}
	return permissions, nil
}

func (t *defaultModelBuildTask) buildCIDRsFromSourceRanges(_ context.Context, ipAddressType elbv2model.IPAddressType, prefixListsConfigured bool) ([]string, error) {
	var cidrs []string
	for _, cidr := range t.service.Spec.LoadBalancerSourceRanges {
		cidrs = append(cidrs, cidr)
	}
	if len(cidrs) == 0 {
		t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSourceRanges, &cidrs, t.service.Annotations)
	}
	for _, cidr := range cidrs {
		if strings.Contains(cidr, ":") && ipAddressType != elbv2model.IPAddressTypeDualStack {
			return nil, errors.Errorf("unsupported v6 cidr %v when lb is not dualstack", cidr)
		}
	}
	if len(cidrs) == 0 {
		if prefixListsConfigured {
			return cidrs, nil
		}
		cidrs = append(cidrs, "0.0.0.0/0")
		if ipAddressType == elbv2model.IPAddressTypeDualStack {
			cidrs = append(cidrs, "::/0")
		}
	}
	return cidrs, nil
}

func (t *defaultModelBuildTask) buildManagedSecurityGroupTags(ctx context.Context) (map[string]string, error) {
	sgTags, err := t.buildAdditionalResourceTags(ctx)
	if err != nil {
		return nil, err
	}
	return algorithm.MergeStringMap(t.defaultTags, sgTags), nil
}
