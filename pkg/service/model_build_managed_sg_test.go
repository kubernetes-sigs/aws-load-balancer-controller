package service

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

func Test_buildCIDRsFromSourceRanges_buildCIDRsFromSourceRanges(t *testing.T) {
	type fields struct {
		svc                   *corev1.Service
		ipAddressType         elbv2model.IPAddressType
		prefixListsConfigured bool
		scheme                elbv2model.LoadBalancerScheme
	}
	tests := []struct {
		name      string
		fields    fields
		setupMock func(MockVPCInfoProvider *networking.MockVPCInfoProvider)
		want      []string
		wantErr   bool
	}{
		{
			name: "default IPv4",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeIPV4,
				prefixListsConfigured: false,
			},
			setupMock: func(MockVPCInfoProvider *networking.MockVPCInfoProvider) {},
			wantErr:   false,
			want: []string{
				"0.0.0.0/0",
			},
		},
		{
			name: "default dualstack",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeDualStack,
				prefixListsConfigured: false,
			},
			setupMock: func(MockVPCInfoProvider *networking.MockVPCInfoProvider) {},
			wantErr:   false,
			want: []string{
				"0.0.0.0/0",
				"::/0",
			},
		},
		{
			name: "no ip range but prefix list",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeDualStack,
				prefixListsConfigured: true,
			},
			setupMock: func(MockVPCInfoProvider *networking.MockVPCInfoProvider) {},
			wantErr:   false,
			want:      nil,
		},
		{
			name: "fetch vpc info for internal scheme",
			fields: fields{
				svc:                   &corev1.Service{},
				ipAddressType:         elbv2model.IPAddressTypeDualStack,
				prefixListsConfigured: false,
				scheme:                elbv2.LoadBalancerSchemeInternal,
			},
			setupMock: func(MockVPCInfoProvider *networking.MockVPCInfoProvider) {
				vpcInfo := networking.VPCInfo{
					CidrBlockAssociationSet: []*ec2sdk.VpcCidrBlockAssociation{
						{
							CidrBlock:      aws.String("192.168.0.0/16"),
							CidrBlockState: &ec2sdk.VpcCidrBlockState{State: aws.String(ec2sdk.VpcCidrBlockStateCodeAssociated)},
						},
					},
					Ipv6CidrBlockAssociationSet: []*ec2sdk.VpcIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock:      aws.String("fd00::/8"),
							Ipv6CidrBlockState: &ec2sdk.VpcCidrBlockState{State: aws.String(ec2sdk.VpcCidrBlockStateCodeAssociated)},
						},
					},
				}
				MockVPCInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), "vpc-1234", gomock.Any()).Return(vpcInfo, nil)
			},
			wantErr: false,
			want: []string{
				"192.168.0.0/16",
				"fd00::/8",
			},
		},
		{
			name: "error fetching vpc info",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-scheme": "internal",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeDualStack,
				prefixListsConfigured: false,
				scheme:                elbv2.LoadBalancerSchemeInternal,
			},
			setupMock: func(MockVPCInfoProvider *networking.MockVPCInfoProvider) {
				MockVPCInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), "vpc-1234", gomock.Any()).Return(networking.VPCInfo{}, errors.New("failed to fetch vpcInfo"))
			},
			wantErr: true,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockVPCInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
			tt.setupMock(mockVPCInfoProvider)
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				service:          tt.fields.svc,
				vpcID:            "vpc-1234",
				vpcInfoProvider:  mockVPCInfoProvider,
			}
			got, err := task.buildCIDRsFromSourceRanges(context.Background(), tt.fields.ipAddressType, tt.fields.prefixListsConfigured, tt.fields.scheme)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_buildCIDRsFromSourceRanges_buildManagedSecurityGroupIngressPermissions(t *testing.T) {
	type fields struct {
		svc           *corev1.Service
		ipAddressType elbv2model.IPAddressType
		scheme        elbv2model.LoadBalancerScheme
	}
	tests := []struct {
		name    string
		fields  fields
		want    []ec2model.IPPermission
		wantErr bool
	}{
		{
			name: "default IPv4",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeIPV4,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
			},
		},
		{
			name: "default dualstack",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "daulstack",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeDualStack,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
				{
					IPProtocol: "",
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(80),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "::/0",
						},
					},
				},
			},
		},
		{
			name: "no ip range but prefix list",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-security-group-prefix-lists": "pl-xxxxx",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
						LoadBalancerSourceRanges: []string{},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeDualStack,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl-xxxxx",
						},
					},
				},
			},
		},
		{
			name: "no ip range but multiple prefix lists",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-security-group-prefix-lists": "pl-xxxxx, pl-yyyyyy",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
						LoadBalancerSourceRanges: []string{},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeDualStack,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl-xxxxx",
						},
					},
				},
				{
					IPProtocol: "",
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl-yyyyyy",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				service:          tt.fields.svc,
			}
			got, err := task.buildManagedSecurityGroupIngressPermissions(context.Background(), tt.fields.ipAddressType, tt.fields.scheme)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}
