package service

import (
	"context"
	"errors"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go/service/ec2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func Test_defaultModelBuilderTask_buildLBAttributes(t *testing.T) {
	tests := []struct {
		testName  string
		svc       *corev1.Service
		wantError bool
		wantValue []elbv2.LoadBalancerAttribute
	}{
		{
			testName: "Default values",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
			},
			wantError: false,
			wantValue: []elbv2.LoadBalancerAttribute{},
		},
		{
			testName: "Annotation specified",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":                "true",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":         "nlb-bucket",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix":       "bkt-pfx",
						"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
						"service.beta.kubernetes.io/aws-load-balancer-attributes":                        "deletion_protection.enabled=true",
					},
				},
			},
			wantError: false,
			wantValue: []elbv2.LoadBalancerAttribute{
				{
					Key:   lbAttrsAccessLogsS3Enabled,
					Value: "true",
				},
				{
					Key:   lbAttrsAccessLogsS3Bucket,
					Value: "nlb-bucket",
				},
				{
					Key:   lbAttrsAccessLogsS3Prefix,
					Value: "bkt-pfx",
				},
				{
					Key:   lbAttrsLoadBalancingCrossZoneEnabled,
					Value: "true",
				},
				{
					Key:   shared_constants.LBAttributeDeletionProtection,
					Value: "true",
				},
			},
		},
		{
			testName: "Attributes from config map annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-attributes": "access_logs.s3.enabled=true,access_logs.s3.bucket=nlb-bucket," +
							"access_logs.s3.prefix=bkt-pfx,load_balancing.cross_zone.enabled=true,deletion_protection.enabled=true,dns_record.client_routing_policy=availability_zone_affinity",
					},
				},
			},
			wantError: false,
			wantValue: []elbv2.LoadBalancerAttribute{
				{
					Key:   lbAttrsAccessLogsS3Enabled,
					Value: "true",
				},
				{
					Key:   lbAttrsAccessLogsS3Bucket,
					Value: "nlb-bucket",
				},
				{
					Key:   lbAttrsAccessLogsS3Prefix,
					Value: "bkt-pfx",
				},
				{
					Key:   lbAttrsLoadBalancingCrossZoneEnabled,
					Value: "true",
				},
				{
					Key:   shared_constants.LBAttributeDeletionProtection,
					Value: "true",
				},
				{
					Key:   lbAttrsLoadBalancingDnsClientRoutingPolicy,
					Value: availabilityZoneAffinity,
				},
			},
		},
		{
			testName: "Specific config overrides config map",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-attributes": "access_logs.s3.enabled=false,access_logs.s3.bucket=nlb-bucket," +
							"access_logs.s3.prefix=bkt-pfx,load_balancing.cross_zone.enabled=true",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":                "true",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":         "overridden-nlb-bucket",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix":       "overridden-bkt-pfx",
						"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "false",
					},
				},
			},
			wantError: false,
			wantValue: []elbv2.LoadBalancerAttribute{
				{
					Key:   lbAttrsAccessLogsS3Enabled,
					Value: "true",
				},
				{
					Key:   lbAttrsAccessLogsS3Bucket,
					Value: "overridden-nlb-bucket",
				},
				{
					Key:   lbAttrsAccessLogsS3Prefix,
					Value: "overridden-bkt-pfx",
				},
				{
					Key:   lbAttrsLoadBalancingCrossZoneEnabled,
					Value: "false",
				},
			},
		},
		{
			testName: "Annotation invalid",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":                "FalSe",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":         "nlb-bucket",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix":       "bkt-pfx",
						"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
					},
				},
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				service:                              tt.svc,
				annotationParser:                     parser,
				defaultAccessLogsS3Bucket:            "",
				defaultAccessLogsS3Prefix:            "",
				defaultLoadBalancingCrossZoneEnabled: false,
				defaultProxyProtocolV2Enabled:        false,
				defaultHealthCheckProtocol:           elbv2.ProtocolTCP,
				defaultHealthCheckPort:               shared_constants.HealthCheckPortTrafficPort,
				defaultHealthCheckPath:               "/",
				defaultHealthCheckInterval:           10,
				defaultHealthCheckTimeout:            10,
				defaultHealthCheckHealthyThreshold:   3,
				defaultHealthCheckUnhealthyThreshold: 3,
			}
			lbAttributes, err := builder.buildLoadBalancerAttributes(context.Background())
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.ElementsMatch(t, tt.wantValue, lbAttributes)
			}
		})
	}
}

func Test_defaultModelBuilderTask_buildSubnetMappings(t *testing.T) {
	tests := []struct {
		name                         string
		ipAddressType                elbv2.IPAddressType
		enablePrefixForIpv6SourceNat elbv2.EnablePrefixForIpv6SourceNat
		scheme                       elbv2.LoadBalancerScheme
		subnets                      []ec2types.Subnet
		want                         []elbv2.SubnetMapping
		svc                          *corev1.Service
		wantErr                      error
	}{
		{
			name:          "ipv4 - with auto-assigned addresses",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{},
			want: []elbv2.SubnetMapping{
				{
					SubnetID: "subnet-1",
				},
				{
					SubnetID: "subnet-2",
				},
			},
		},
		{
			name:          "ipv4 - with EIP allocation",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-eip-allocations": "eip1, eip2",
					},
				},
			},
			want: []elbv2.SubnetMapping{
				{
					SubnetID:     "subnet-1",
					AllocationID: aws.String("eip1"),
				},
				{
					SubnetID:     "subnet-2",
					AllocationID: aws.String("eip2"),
				},
			},
		},
		{
			name:          "ipv4 - with EIP allocation: on internal load balancer",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-eip-allocations": "eip1, eip2",
					},
				},
			},
			wantErr: errors.New("EIP allocations can only be set for internet facing load balancers"),
		},
		{
			name:          "ipv4 - with EIP allocation: subnet count mismatch",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-eip-allocations": "eip1",
					},
				},
			},
			wantErr: errors.New("count of EIP allocations (1) and subnets (2) must match"),
		},
		{
			name:          "ipv4 - with PrivateIPv4Address",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "192.168.2.1, 192.168.1.1",
					},
				},
			},
			want: []elbv2.SubnetMapping{
				{
					SubnetID:           "subnet-1",
					PrivateIPv4Address: aws.String("192.168.1.1"),
				},
				{
					SubnetID:           "subnet-2",
					PrivateIPv4Address: aws.String("192.168.2.1"),
				},
			},
		},
		{
			name:          "ipv4 - with PrivateIPv4Address: on internet-facing load balancer",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "192.168.1.1, 192.168.2.1",
					},
				},
			},
			wantErr: errors.New("private IPv4 addresses can only be set for internal load balancers"),
		},
		{
			name:          "ipv4 - with PrivateIpv4Addresses: subnet count mismatch",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "172.16.1.1",
					},
				},
			},
			wantErr: errors.New("count of private IPv4 addresses (1) and subnets (2) must match"),
		},
		{
			name:          "ipv4 - with PrivateIPv4Address: no matching IP for subnets",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "192.168.1.1, 192.168.3.1",
					},
				},
			},
			wantErr: errors.New("expect one private IPv4 address configured for subnet: subnet-2"),
		},
		{
			name:          "ipv4 - with PrivateIPv4Address",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "192.168.2.1, 192.168.1.1",
					},
				},
			},
			want: []elbv2.SubnetMapping{
				{
					SubnetID:           "subnet-1",
					PrivateIPv4Address: aws.String("192.168.1.1"),
				},
				{
					SubnetID:           "subnet-2",
					PrivateIPv4Address: aws.String("192.168.2.1"),
				},
			},
		},
		{
			name:          "ipv4 - with PrivateIPv4Address: invalid ip format",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "192.168.1.1, i-am-not-an-ip",
					},
				},
			},
			wantErr: errors.New("private IPv4 addresses must be valid IP address: i-am-not-an-ip"),
		},
		{
			name:          "ipv4 - with PrivateIPv4Address: invalid ipv4 format",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "192.168.1.1, 2600:1f13:837:8500::1",
					},
				},
			},
			wantErr: errors.New("private IPv4 addresses must be valid IPv4 address: 2600:1f13:837:8500::1"),
		},
		{
			name:          "dualstack - with IPv6Addresses",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses": "2600:1f13:837:8504::1, 2600:1f13:837:8500::1",
					},
				},
			},
			want: []elbv2.SubnetMapping{
				{
					SubnetID:    "subnet-1",
					IPv6Address: aws.String("2600:1f13:837:8500::1"),
				},
				{
					SubnetID:    "subnet-2",
					IPv6Address: aws.String("2600:1f13:837:8504::1"),
				},
			},
		},
		{
			name:          "dualstack - with IPv6Addresses: on a ipv4 load balancer",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses": "2600:1f13:837:8504::1, 2600:1f13:837:8500::1",
					},
				},
			},
			wantErr: errors.New("IPv6 addresses can only be set for dualstack load balancers"),
		},
		{
			name:          "dualstack - with IPv6Addresses: subnet count mismatch",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses": "2600:1f13:837:8504::1",
					},
				},
			},
			wantErr: errors.New("count of IPv6 addresses (1) and subnets (2) must match"),
		},
		{
			name:          "dualstack - with IPv6Addresses: no matching IP for subnets",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses": "2600:1f13:837:8500::1, 2600:1f13:837:8508::1",
					},
				},
			},
			wantErr: errors.New("expect one IPv6 address configured for subnet: subnet-2"),
		},
		{
			name:          "dualstack - with IPv6Addresses: invalid IP format",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses": "2600:1f13:837:8504::1, i-am-not-an-ip",
					},
				},
			},
			wantErr: errors.New("IPv6 addresses must be valid IP address: i-am-not-an-ip"),
		},
		{
			name:          "dualstack - with IPv6Addresses: invalid IP format",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses": "2600:1f13:837:8504::1, 192.168.1.1",
					},
				},
			},
			wantErr: errors.New("IPv6 addresses must be valid IPv6 address: 192.168.1.1"),
		},
		{
			name:          "dualstack - with EIPAllocation and IPv6Addresses",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-eip-allocations": "eip1, eip2",
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses":  "2600:1f13:837:8504::1, 2600:1f13:837:8500::1",
					},
				},
			},
			want: []elbv2.SubnetMapping{
				{
					SubnetID:     "subnet-1",
					AllocationID: aws.String("eip1"),
					IPv6Address:  aws.String("2600:1f13:837:8500::1"),
				},
				{
					SubnetID:     "subnet-2",
					AllocationID: aws.String("eip2"),
					IPv6Address:  aws.String("2600:1f13:837:8504::1"),
				},
			},
		},
		{
			name:          "dualstack - with EIPAllocation and IPv6Addresses",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			scheme:        elbv2.LoadBalancerSchemeInternal,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2types.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-private-ipv4-addresses": "192.168.2.1, 192.168.1.1",
						"service.beta.kubernetes.io/aws-load-balancer-ipv6-addresses":         "2600:1f13:837:8504::1, 2600:1f13:837:8500::1",
					},
				},
			},
			want: []elbv2.SubnetMapping{
				{
					SubnetID:           "subnet-1",
					PrivateIPv4Address: aws.String("192.168.1.1"),
					IPv6Address:        aws.String("2600:1f13:837:8500::1"),
				},
				{
					SubnetID:           "subnet-2",
					PrivateIPv4Address: aws.String("192.168.2.1"),
					IPv6Address:        aws.String("2600:1f13:837:8504::1"),
				},
			},
		},
		{
			name:                         "dualstack - source-nat-ipv6-prefixes - should throw error if enable-prefix-for-ipv6-source-nat is not provided or is off, but still source-nat-ipv6-prefixes is provided",
			ipAddressType:                elbv2.IPAddressTypeDualStack,
			scheme:                       elbv2.LoadBalancerSchemeInternal,
			enablePrefixForIpv6SourceNat: elbv2.EnablePrefixForIpv6SourceNatOff,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-source-nat-ipv6-prefixes": "2600:1f13:837:8504::1/80",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("source-nat-ipv6-prefixes annotation is only applicable if enable-prefix-for-ipv6-source-nat annotation is set to on."),
		},
		{
			name:                         "dualstack - source-nat-ipv6-prefixes - should throw error if its not dualstack nlb, but source-nat-ipv6-prefixes is set",
			ipAddressType:                elbv2.IPAddressTypeIPV4,
			scheme:                       elbv2.LoadBalancerSchemeInternal,
			enablePrefixForIpv6SourceNat: elbv2.EnablePrefixForIpv6SourceNatOn,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-source-nat-ipv6-prefixes": "2600:1f13:837:8504::1/80",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("source-nat-ipv6-prefixes annotation can only be set for Network Load Balancers using Dualstack IP address type."),
		},
		{
			name:                         "dualstack - source-nat-ipv6-prefixes - should throw error if source-nat-ipv6-prefix is not a valid IPv6 CIDR -1",
			ipAddressType:                elbv2.IPAddressTypeDualStack,
			scheme:                       elbv2.LoadBalancerSchemeInternal,
			enablePrefixForIpv6SourceNat: elbv2.EnablePrefixForIpv6SourceNatOn,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-source-nat-ipv6-prefixes": "2600:1f13:837:8766:6766:7987:6666:1:999/80",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("Invalid value in source-nat-ipv6-prefixes: 2600:1f13:837:8766:6766:7987:6666:1:999/80. Value must be a valid IPv6 CIDR."),
		},
		{
			name:                         "dualstack - source-nat-ipv6-prefixes - should throw error if source-nat-ipv6-prefix is not a valid IPv6 CIDR -2",
			ipAddressType:                elbv2.IPAddressTypeDualStack,
			scheme:                       elbv2.LoadBalancerSchemeInternal,
			enablePrefixForIpv6SourceNat: elbv2.EnablePrefixForIpv6SourceNatOn,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-source-nat-ipv6-prefixes": "2600:1f13:837:87667::/80",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("Invalid value in source-nat-ipv6-prefixes: 2600:1f13:837:87667::/80. Value must be a valid IPv6 CIDR."),
		},
		{
			name:                         "dualstack - source-nat-ipv6-prefixes - should throw error if source-nat-ipv6-prefix is not a valid IPv6 CIDR -3",
			ipAddressType:                elbv2.IPAddressTypeDualStack,
			scheme:                       elbv2.LoadBalancerSchemeInternal,
			enablePrefixForIpv6SourceNat: elbv2.EnablePrefixForIpv6SourceNatOn,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-source-nat-ipv6-prefixes": "2600:1f13:837:8766:77:789:9:9::/80",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("Invalid value in source-nat-ipv6-prefixes: 2600:1f13:837:8766:77:789:9:9::/80. Value must be a valid IPv6 CIDR."),
		},
		{
			name:                         "dualstack - source-nat-ipv6-prefixes - should throw error if source-nat-ipv6-prefix within subnet CIDR range",
			ipAddressType:                elbv2.IPAddressTypeDualStack,
			scheme:                       elbv2.LoadBalancerSchemeInternal,
			enablePrefixForIpv6SourceNat: elbv2.EnablePrefixForIpv6SourceNatOn,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-source-nat-ipv6-prefixes": "2601:1f13:837:8500:009::/80",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("Invalid value in source-nat-ipv6-prefixes: 2601:1f13:837:8500:009::/80. Value must be within subnet CIDR range: [2600:1f13:837:8500::/64]."),
		},
		{
			name:                         "dualstack - source-nat-ipv6-prefixes - should throw error if source-nat-ipv6-prefix doesnt have allowed prefix length of 80",
			ipAddressType:                elbv2.IPAddressTypeDualStack,
			scheme:                       elbv2.LoadBalancerSchemeInternal,
			enablePrefixForIpv6SourceNat: elbv2.EnablePrefixForIpv6SourceNatOn,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-source-nat-ipv6-prefixes": "2600:1f13:837:8500:9::/70",
					},
				},
			},
			want:    nil,
			wantErr: errors.New("Invalid value in source-nat-ipv6-prefixes: 2600:1f13:837:8500:9::/70. Prefix length must be 80, but 70 is specified."),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{service: tt.svc, annotationParser: annotationParser}
			got, err := builder.buildLoadBalancerSubnetMappings(context.Background(), tt.ipAddressType, tt.scheme, tt.subnets, tt.enablePrefixForIpv6SourceNat)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuilderTask_buildLoadBalancerSubnets(t *testing.T) {
	type resolveSubnetResults struct {
		subnets []ec2types.Subnet
		err     error
	}
	type args struct {
		stack core.Stack
	}
	type listLoadBalancerCall struct {
		sdkLBs []elbv2deploy.LoadBalancerWithTags
		err    error
	}
	listLoadBalancerCallForEmptyLB := listLoadBalancerCall{
		sdkLBs: []elbv2deploy.LoadBalancerWithTags{},
	}
	tests := []struct {
		name                         string
		svc                          *corev1.Service
		scheme                       elbv2.LoadBalancerScheme
		provider                     tracking.Provider
		args                         args
		listLoadBalancersCalls       []listLoadBalancerCall
		resolveViaDiscoveryCalls     []resolveSubnetResults
		resolveViaNameOrIDSliceCalls []resolveSubnetResults
		want                         []ec2types.Subnet
		wantErr                      error
	}{
		{
			name:                   "subnet auto-discovery",
			svc:                    &corev1.Service{},
			scheme:                 elbv2.LoadBalancerSchemeInternal,
			provider:               tracking.NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:                   args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			listLoadBalancersCalls: []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			resolveViaDiscoveryCalls: []resolveSubnetResults{
				{
					subnets: []ec2types.Subnet{
						{
							SubnetId:  aws.String("subnet-a"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
						{
							SubnetId:  aws.String("subnet-b"),
							CidrBlock: aws.String("192.168.32.0/19"),
						},
					},
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:  aws.String("subnet-a"),
					CidrBlock: aws.String("192.168.0.0/19"),
				},
				{
					SubnetId:  aws.String("subnet-b"),
					CidrBlock: aws.String("192.168.32.0/19"),
				},
			},
		},
		{
			name: "subnet annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-subnets": "subnet-abc, Subnet Name XYZ",
					},
				},
			},
			scheme:   elbv2.LoadBalancerSchemeInternal,
			provider: tracking.NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			resolveViaNameOrIDSliceCalls: []resolveSubnetResults{
				{
					subnets: []ec2types.Subnet{
						{
							SubnetId:  aws.String("subnet-abc"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
						{
							SubnetId:  aws.String("subnet-xyz"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
					},
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:  aws.String("subnet-abc"),
					CidrBlock: aws.String("192.168.0.0/19"),
				},
				{
					SubnetId:  aws.String("subnet-xyz"),
					CidrBlock: aws.String("192.168.0.0/19"),
				},
			},
		},
		{
			name:     "subnet resolve via Name or ID, with existing LB and scheme wouldn't change",
			svc:      &corev1.Service{},
			scheme:   elbv2.LoadBalancerSchemeInternal,
			provider: tracking.NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			listLoadBalancersCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2deploy.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2types.LoadBalancer{
								LoadBalancerArn: aws.String("lb-1"),
								AvailabilityZones: []elbv2types.AvailabilityZone{
									{
										SubnetId: aws.String("subnet-c"),
									},
									{
										SubnetId: aws.String("subnet-d"),
									},
								},
								Scheme: elbv2types.LoadBalancerSchemeEnumInternal,
							},
							Tags: map[string]string{
								shared_constants.TagKeyK8sCluster: "cluster-name",
								"service.k8s.aws/stack":           "namespace/serviceName",
							},
						},
					},
				},
			},
			resolveViaNameOrIDSliceCalls: []resolveSubnetResults{
				{
					subnets: []ec2types.Subnet{
						{
							SubnetId:  aws.String("subnet-c"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
						{
							SubnetId:  aws.String("subnet-d"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
					},
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:  aws.String("subnet-c"),
					CidrBlock: aws.String("192.168.0.0/19"),
				},
				{
					SubnetId:  aws.String("subnet-d"),
					CidrBlock: aws.String("192.168.0.0/19"),
				},
			},
		},
		{
			name:     "subnet auto discovery, with existing LB and scheme would change",
			svc:      &corev1.Service{},
			scheme:   elbv2.LoadBalancerSchemeInternal,
			provider: tracking.NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			listLoadBalancersCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2deploy.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2types.LoadBalancer{
								LoadBalancerArn: aws.String("lb-1"),
								AvailabilityZones: []elbv2types.AvailabilityZone{
									{
										SubnetId: aws.String("subnet-c"),
									},
									{
										SubnetId: aws.String("subnet-d"),
									},
								},
								Scheme: elbv2types.LoadBalancerSchemeEnumInternetFacing,
							},
							Tags: map[string]string{
								"elbv2.k8s.aws/cluster": "cluster-name",
								"service.k8s.aws/stack": "namespace/serviceName",
							},
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveSubnetResults{
				{
					subnets: []ec2types.Subnet{
						{
							SubnetId:  aws.String("subnet-a"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
						{
							SubnetId:  aws.String("subnet-b"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
					},
				},
			},
			want: []ec2types.Subnet{
				{
					SubnetId:  aws.String("subnet-a"),
					CidrBlock: aws.String("192.168.0.0/19"),
				},
				{
					SubnetId:  aws.String("subnet-b"),
					CidrBlock: aws.String("192.168.0.0/19"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			elbv2TaggingManager := elbv2deploy.NewMockTaggingManager(ctrl)
			for _, call := range tt.listLoadBalancersCalls {
				elbv2TaggingManager.EXPECT().ListLoadBalancers(gomock.Any(), gomock.Any()).Return(call.sdkLBs, call.err)
			}

			subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
			for _, call := range tt.resolveViaDiscoveryCalls {
				subnetsResolver.EXPECT().ResolveViaDiscovery(gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}
			for _, call := range tt.resolveViaNameOrIDSliceCalls {
				subnetsResolver.EXPECT().ResolveViaNameOrIDSlice(gomock.Any(), gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")

			clusterName := "cluster-name"
			trackingProvider := tracking.NewDefaultProvider("ingress.k8s.aws", clusterName)
			featureGates := config.NewFeatureGates()

			builder := &defaultModelBuildTask{
				clusterName:         clusterName,
				service:             tt.svc,
				stack:               tt.args.stack,
				annotationParser:    annotationParser,
				subnetsResolver:     subnetsResolver,
				trackingProvider:    trackingProvider,
				elbv2TaggingManager: elbv2TaggingManager,
				featureGates:        featureGates,
			}
			got, err := builder.buildLoadBalancerSubnets(context.Background(), tt.scheme)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildLoadBalancerIPAddressType(t *testing.T) {
	tests := []struct {
		name    string
		service *corev1.Service
		want    elbv2.IPAddressType
		wantErr bool
	}{
		{
			name: "ipv4_specified_expect_ipv4",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "ipv4"},
				},
			},
			want:    elbv2.IPAddressTypeIPV4,
			wantErr: false,
		},
		{
			name: "dualstack_specified_expect_dualstack",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack"},
				},
			},
			want:    elbv2.IPAddressTypeDualStack,
			wantErr: false,
		},
		{
			name: "default_value_no_ip_address_type_specified_expect_ipv4",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-other-annotation": "somevalue"},
				},
			},
			want:    elbv2.IPAddressTypeIPV4,
			wantErr: false,
		},
		{
			name: "invalid_value_expect_error",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "DualStack"},
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser:     parser,
				service:              tt.service,
				defaultIPAddressType: elbv2.IPAddressTypeIPV4,
			}

			got, err := builder.buildLoadBalancerIPAddressType(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("buildLoadBalancerIPAddressType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("buildLoadBalancerIPAddressType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildLoadBalancerEnablePrefixForIpv6SourceNat(t *testing.T) {
	tests := []struct {
		name          string
		subnets       []ec2types.Subnet
		ipAddressType elbv2.IPAddressType
		service       *corev1.Service
		want          elbv2.EnablePrefixForIpv6SourceNat
		wantErr       error
	}{
		{
			name:          "should error out if EnablePrefixForIpv6SourceNat is set to on for ipv4 address Type NLB",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				}},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-enable-prefix-for-ipv6-source-nat": elbv2.ON},
				},
			},
			want:    "",
			wantErr: errors.New("enable-prefix-for-ipv6-source-nat annotation is only applicable to Network Load Balancers using Dualstack IP address type."),
		},
		{
			name:          "should error out if EnablePrefixForIpv6SourceNat is set to on for dualstack NLB which doesnt have ipv6 Cidr subnet",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				}},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-enable-prefix-for-ipv6-source-nat": elbv2.ON},
				},
			},
			want:    "",
			wantErr: errors.New("To enable prefix for source NAT, all associated subnets must have an IPv6 CIDR. Subnets without IPv6 CIDR: [subnet-1]."),
		},
		{
			name:          "should error out if EnablePrefixForIpv6SourceNat value is set to something else than allowed values on or off",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
				}},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-enable-prefix-for-ipv6-source-nat": "randomValue"},
				},
			},
			want:    "",
			wantErr: errors.New("Invalid enable-prefix-for-ipv6-source-nat value: randomValue. Valid values are ['on', 'off']."),
		},
		{
			name:          "should return EnablePrefixForIpv6SourceNat as on if annotation value is on",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				}},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-enable-prefix-for-ipv6-source-nat": elbv2.ON},
				},
			},
			want:    elbv2.ON,
			wantErr: nil,
		},

		{
			name:          "should return EnablePrefixForIpv6SourceNat as off if annotation value is off",
			ipAddressType: elbv2.IPAddressTypeDualStack,
			subnets: []ec2types.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
								State: ec2.SubnetCidrBlockStateCodeAssociated,
							},
						},
					},
				}},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-enable-prefix-for-ipv6-source-nat": elbv2.OFF},
				},
			},
			want:    elbv2.OFF,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser:     parser,
				service:              tt.service,
				defaultIPAddressType: elbv2.IPAddressTypeIPV4,
			}

			got, err := builder.buildLoadBalancerEnablePrefixForIpv6SourceNat(context.Background(), tt.ipAddressType, tt.subnets)
			if err != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}
			if got != tt.want {
				t.Errorf("buildLoadBalancerIPAddressType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildAdditionalResourceTags(t *testing.T) {
	type fields struct {
		service             *corev1.Service
		defaultTags         map[string]string
		externalManagedTags sets.String
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]string
		wantErr error
	}{
		{
			name: "empty default tags, no tags annotation",
			fields: fields{
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{},
		},
		{
			name: "empty default tags, non-empty tags annotation",
			fields: fields{
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2",
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non-empty default tags, empty tags annotation",
			fields: fields{
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "non-empty default tags, non-empty tags annotation",
			fields: fields{
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3a",
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "non-empty external tags, non-empty tags annotation - no collision",
			fields: fields{
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3a",
						},
					},
				},
				externalManagedTags: sets.NewString("k4"),
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3a",
			},
		},
		{
			name: "non-empty external tags, non-empty tags annotation - has collision",
			fields: fields{
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3a",
						},
					},
				},
				externalManagedTags: sets.NewString("k3", "k4"),
			},
			wantErr: errors.New("external managed tag key k3 cannot be specified on Service"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				service:             tt.fields.service,
				defaultTags:         tt.fields.defaultTags,
				externalManagedTags: tt.fields.externalManagedTags,
				annotationParser:    annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
			}
			got, err := task.buildAdditionalResourceTags(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildLoadBalancerName(t *testing.T) {
	tests := []struct {
		name        string
		service     *corev1.Service
		clusterName string
		scheme      elbv2.LoadBalancerScheme
		want        string
		wantErr     error
	}{
		{
			name: "no name annotation",

			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "foo",
					Name:        "bar",
					Annotations: map[string]string{},
				},
			},
			scheme: elbv2.LoadBalancerSchemeInternetFacing,
			want:   "k8s-foo-bar-e053368fb2",
		},
		{
			name: "non-empty name annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-name": "baz",
					},
				},
			},
			want: "baz",
		},
		{
			name: "reject name longer than 32 characters",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-name": "bazbazfoofoobazbazfoofoobazbazfoo",
					},
				},
			},
			wantErr: errors.New("load balancer name cannot be longer than 32 characters"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				service:          tt.service,
				clusterName:      tt.clusterName,
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
			}
			got, err := task.buildLoadBalancerName(context.Background(), tt.scheme)
			if err != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuilderTask_buildLbCapacity(t *testing.T) {
	tests := []struct {
		testName  string
		svc       *corev1.Service
		wantError bool
		wantValue *elbv2.MinimumLoadBalancerCapacity
	}{
		{
			testName: "Default value",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
			},
			wantError: false,
			wantValue: nil,
		},
		{
			testName: "Annotation specified",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                           "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-minimum-load-balancer-capacity": "CapacityUnits=3000",
					},
				},
			},
			wantError: false,
			wantValue: &elbv2.MinimumLoadBalancerCapacity{
				CapacityUnits: int32(3000),
			},
		},
		{
			testName: "Annotation invalid",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                           "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-minimum-load-balancer-capacity": "InvalidUnits=3000",
					},
				},
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			featureGates := config.NewFeatureGates()
			builder := &defaultModelBuildTask{
				service:                              tt.svc,
				annotationParser:                     parser,
				defaultAccessLogsS3Bucket:            "",
				defaultAccessLogsS3Prefix:            "",
				defaultLoadBalancingCrossZoneEnabled: false,
				defaultProxyProtocolV2Enabled:        false,
				defaultHealthCheckProtocol:           elbv2.ProtocolTCP,
				defaultHealthCheckPort:               shared_constants.HealthCheckPortTrafficPort,
				defaultHealthCheckPath:               "/",
				defaultHealthCheckInterval:           10,
				defaultHealthCheckTimeout:            10,
				defaultHealthCheckHealthyThreshold:   3,
				defaultHealthCheckUnhealthyThreshold: 3,
				featureGates:                         featureGates,
			}
			lbMinimumCapacity, err := builder.buildLoadBalancerMinimumCapacity(context.Background())
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.wantValue, lbMinimumCapacity)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildManageSecurityGroupRulesFlag(t *testing.T) {
	tests := []struct {
		name                       string
		enableManageBackendSGRules bool
		annotations                map[string]string
		wantManageSGRules          bool
		wantErr                    bool
	}{
		{
			name:                       "with no annotation and enableManageBackendSGRules=false - expect enable manage security group rules to be false",
			enableManageBackendSGRules: false,
			annotations:                map[string]string{},
			wantManageSGRules:          false,
			wantErr:                    false,
		},
		{
			name:                       "with no annotation and enableManageBackendSGRules=true - expect enable manage security group rules to be true",
			enableManageBackendSGRules: true,
			annotations:                map[string]string{},
			wantManageSGRules:          true,
			wantErr:                    false,
		},
		{
			name:                       "with annotation true and enableManageBackendSGRules=false - expect override and enable manage security group rules to be true",
			enableManageBackendSGRules: false,
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "true",
			},
			wantManageSGRules: true,
			wantErr:           false,
		},
		{
			name:                       "with annotation false and enableManageBackendSGRules=true - expect override and enable manage security group rules to be false",
			enableManageBackendSGRules: true,
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "false",
			},
			wantManageSGRules: false,
			wantErr:           false,
		},
		{
			name:                       "with invalid annotation - expect error",
			enableManageBackendSGRules: false,
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "invalid",
			},
			wantManageSGRules: false,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")

			task := &defaultModelBuildTask{
				enableManageBackendSGRules: tt.enableManageBackendSGRules,
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: tt.annotations,
					},
				},
				annotationParser: annotationParser,
			}

			got, err := task.buildManageSecurityGroupRulesFlag(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("buildManageSecurityGroupRulesFlag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantManageSGRules {
				t.Errorf("buildManageSecurityGroupRulesFlag() got = %v, want %v", got, tt.wantManageSGRules)
			}
		})
	}
}
