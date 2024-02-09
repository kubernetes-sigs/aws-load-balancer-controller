package service

import (
	"context"
	"errors"
	"testing"

	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
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

const lbAttrsDeletionProtectionEnabled = "deletion_protection.enabled"

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
					Key:   lbAttrsDeletionProtectionEnabled,
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
							"access_logs.s3.prefix=bkt-pfx,load_balancing.cross_zone.enabled=true,deletion_protection.enabled=true",
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
					Key:   lbAttrsDeletionProtectionEnabled,
					Value: "true",
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
				defaultHealthCheckPort:               healthCheckPortTrafficPort,
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
		name          string
		ipAddressType elbv2.IPAddressType
		scheme        elbv2.LoadBalancerScheme
		subnets       []*ec2.Subnet
		want          []elbv2.SubnetMapping
		svc           *corev1.Service
		wantErr       error
	}{
		{
			name:          "ipv4 - with auto-assigned addresses",
			ipAddressType: elbv2.IPAddressTypeIPV4,
			scheme:        elbv2.LoadBalancerSchemeInternetFacing,
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
			subnets: []*ec2.Subnet{
				{
					SubnetId:         aws.String("subnet-1"),
					AvailabilityZone: aws.String("us-west-2a"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
				{
					SubnetId:         aws.String("subnet-2"),
					AvailabilityZone: aws.String("us-west-2b"),
					VpcId:            aws.String("vpc-1"),
					CidrBlock:        aws.String("192.168.2.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2.SubnetCidrBlockState{
								State: aws.String(ec2.SubnetCidrBlockStateCodeAssociated),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{service: tt.svc, annotationParser: annotationParser}
			got, err := builder.buildLoadBalancerSubnetMappings(context.Background(), tt.ipAddressType, tt.scheme, tt.subnets)
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
		subnets []*ec2.Subnet
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
		want                         []*ec2.Subnet
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
					subnets: []*ec2.Subnet{
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
			want: []*ec2.Subnet{
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
					subnets: []*ec2.Subnet{
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
			want: []*ec2.Subnet{
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
							LoadBalancer: &elbv2sdk.LoadBalancer{
								LoadBalancerArn: aws.String("lb-1"),
								AvailabilityZones: []*elbv2sdk.AvailabilityZone{
									{
										SubnetId: aws.String("subnet-c"),
									},
									{
										SubnetId: aws.String("subnet-d"),
									},
								},
								Scheme: aws.String("internal"),
							},
							Tags: map[string]string{
								"elbv2.k8s.aws/cluster": "cluster-name",
								"service.k8s.aws/stack": "namespace/serviceName",
							},
						},
					},
				},
			},
			resolveViaNameOrIDSliceCalls: []resolveSubnetResults{
				{
					subnets: []*ec2.Subnet{
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
			want: []*ec2.Subnet{
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
							LoadBalancer: &elbv2sdk.LoadBalancer{
								LoadBalancerArn: aws.String("lb-1"),
								AvailabilityZones: []*elbv2sdk.AvailabilityZone{
									{
										SubnetId: aws.String("subnet-c"),
									},
									{
										SubnetId: aws.String("subnet-d"),
									},
								},
								Scheme: aws.String("internet-facing"),
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
					subnets: []*ec2.Subnet{
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
			want: []*ec2.Subnet{
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
