package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

const (
	resourceIDManagedSecurityGroup = "ManagedLBSecurityGroup"
)

func (t *defaultModelBuildTask) buildManagedSecurityGroup(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig, ipAddressType elbv2model.IPAddressType) (*ec2model.SecurityGroup, error) {
	sgSpec, err := t.buildManagedSecurityGroupSpec(ctx, listenPortConfigByPort, ipAddressType)
	if err != nil {
		return nil, err
	}

	sg := ec2model.NewSecurityGroup(t.stack, resourceIDManagedSecurityGroup, sgSpec)
	return sg, nil
}

func (t *defaultModelBuildTask) buildManagedSecurityGroupSpec(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig, ipAddressType elbv2model.IPAddressType) (ec2model.SecurityGroupSpec, error) {
	name := t.buildManagedSecurityGroupName(ctx)
	tags, err := t.buildManagedSecurityGroupTags(ctx)
	if err != nil {
		return ec2model.SecurityGroupSpec{}, err
	}
	ingressPermissions := t.buildManagedSecurityGroupIngressPermissions(ctx, listenPortConfigByPort, ipAddressType)
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
	_, _ = uuidHash.Write([]byte(t.ingGroup.ID.String()))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	if t.ingGroup.ID.IsExplicit() {
		payload := invalidSecurityGroupNamePtn.ReplaceAllString(t.ingGroup.ID.Name, "")
		return fmt.Sprintf("k8s-%.17s-%.10s", payload, uuid)
	}

	sanitizedNamespace := invalidSecurityGroupNamePtn.ReplaceAllString(t.ingGroup.ID.Namespace, "")
	sanitizedName := invalidSecurityGroupNamePtn.ReplaceAllString(t.ingGroup.ID.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildManagedSecurityGroupTags(_ context.Context) (map[string]string, error) {
	ingGroupTags, err := t.buildIngressGroupResourceTags(t.ingGroup.Members)
	if err != nil {
		return nil, err
	}
	return algorithm.MergeStringMap(t.defaultTags, ingGroupTags), nil
}

func (t *defaultModelBuildTask) buildManagedSecurityGroupIngressPermissions(_ context.Context, listenPortConfigByPort map[int64]listenPortConfig, ipAddressType elbv2model.IPAddressType) []ec2model.IPPermission {
	var permissions []ec2model.IPPermission
	for port, cfg := range listenPortConfigByPort {
		for _, cidr := range cfg.inboundCIDRv4s {
			permissions = append(permissions, ec2model.IPPermission{
				IPProtocol: "tcp",
				FromPort:   awssdk.Int64(port),
				ToPort:     awssdk.Int64(port),
				IPRanges: []ec2model.IPRange{
					{
						CIDRIP: cidr,
					},
				},
			})
		}
		if ipAddressType == elbv2model.IPAddressTypeDualStack {
			for _, cidr := range cfg.inboundCIDRv6s {
				permissions = append(permissions, ec2model.IPPermission{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int64(port),
					ToPort:     awssdk.Int64(port),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: cidr,
						},
					},
				})
			}
		}
	}
	return permissions
}
